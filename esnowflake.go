/*
Random
* 1                                             41                           65                                                                       128
* +---------------------------------------------+----------------------------+--------------------------------------------------------------------------+
* | timestamp(ms)                                | worker info                | random number                                                            |
* +---------------------------------------------+----------------------------+--------------------------------------------------------------------------+
* | 00000000 00000000 00000000 00000000 00000000 | 00000000 00000000 00000000 | 00000000 00000000 00000000 00000000 00000000 00000000 | 00000000 00000000 |
* +---------------------------------------------+----------------------------+--------------------------------------------------------------------------+
*
* 1. 40 位时间截(毫秒级)，注意这是时间截的差值（当前时间截 - 开始时间截)。可以使用约 34 年: (1L << 40) / (1000L * 60 * 60 * 24 * 365) = 34。（2020-2054）
* 2. 24 位 worker info 数据，适应 k8s 环境。
* 3. 64 随机数
*/

/*
Sequence
* 1                                             41                           65                                                     113                128
* +---------------------------------------------+----------------------------+------------------------------------------------------+-------------------+
* | timestamp(ms)                                | worker info                | random number                                        | sequence          |
* +---------------------------------------------+----------------------------+------------------------------------------------------+-------------------+
* | 00000000 00000000 00000000 00000000 00000000 | 00000000 00000000 00000000 | 00000000 00000000 00000000 00000000 00000000 00000000 | 00000000 00000000 |
* +---------------------------------------------+----------------------------+------------------------------------------------------+-------------------+
*
* 1. 40 位时间截(毫秒级)，注意这是时间截的差值（当前时间截 - 开始时间截)。可以使用约 34 年: (1L << 40) / (1000L * 60 * 60 * 24 * 365) = 34。（2020-2054）
* 2. 24 位 worker info 数据，适应 k8s 环境。
* 3. 48 随机数
* 4. 16 位 sequence
*/

package esnowflake

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	twepoch        = int64(1640995200000) // 开始时间截 (2022-01-01)
	workerInfoBits = uint(24)             // 机器 ip 所占的位数
)

const randPoolSequenceRandomSize = 6 * 256
const randPoolRandomSize = 8 * 64 * 3

var (
	rander                = rand.Reader                      // random function
	poolSequenceRandomPos = randPoolSequenceRandomSize       // protected with poolMu
	poolSequenceRandom    [randPoolSequenceRandomSize]byte   // protected with poolMu
	poolRandomPos         = randPoolRandomSize               // protected with poolMu
	poolRandom            [randPoolRandomSize]byte           // protected with poolMu
	sequenceBits          = uint(16)                         // 序列所占的位数
	sequenceMask          = int64(-1 ^ (-1 << sequenceBits)) //

)

type Config struct {
	sync.Mutex
	Mask1     uint8
	Mask2     uint8
	Mask3     uint8
	ip        []byte
	timestamp int64
	sequence  int64
}

// New create a new snowflake node with a unique worker id.
// ip 你的机器 ip
// mask1, mask2, mask3 你自己填的随机数，用于混淆 ip
func New(ip string, mask1, mask2, mask3 uint8) *Config {
	b := net.ParseIP(ip).To4()
	if b == nil {
		panic("invalid ipv4 format")
	}
	obj := Config{
		Mask1: mask1,
		Mask2: mask2,
		Mask3: mask3,
	}
	b[1] = b[1] ^ obj.Mask1
	b[2] = b[2] ^ obj.Mask2
	b[3] = b[3] ^ obj.Mask3
	obj.ip = b[1:]
	return &obj
}

// GenerateByRandom 生成一个唯一的 id， 这个性能比较好，只有非常低的被碰撞概率，我们认为可以忽略不计
func (s *Config) GenerateByRandom() string {
	buf := make([]byte, 16)
	s.Lock()
	now := time.Now().UnixNano() / 1000000
	if poolRandomPos == randPoolRandomSize {
		// 生成48个字节的随机数
		// 下面在buf中，每次取8个字节
		// 如果 poolRandomPos 到达最大值，重新生成随机数，并将 poolRandomPos 置为 0
		_, err := io.ReadFull(rander, poolRandom[:])
		if err != nil {
			s.Unlock()
			panic(err)
		}
		poolRandomPos = 0
	}
	copy(buf[:5], Uint64ToBytes(uint64(now-twepoch) << workerInfoBits)[:5])
	copy(buf[5:8], s.ip)
	copy(buf[8:], poolRandom[poolRandomPos:(poolRandomPos+8)])
	poolRandomPos += 8
	s.Unlock()
	return base64.RawURLEncoding.EncodeToString(buf)
}

func (s *Config) GenerateBySequence() string {
	buf := make([]byte, 16)
	s.Lock()
	now := time.Now().UnixNano() / 1000000
	if s.timestamp == now {
		s.sequence = (s.sequence + 1) & sequenceMask
		if s.sequence == 0 {
			for now <= s.timestamp {
				now = time.Now().UnixNano() / 1000000
			}
		}
	} else {
		s.sequence = 0
	}
	s.timestamp = now

	// 生成48个字节的随机数
	// 下面在buf中，每次取6个字节
	// 如果 poolSequenceRandomPos 到达最大值，重新生成随机数，并将 poolSequenceRandomPos 置为 0
	if poolSequenceRandomPos == randPoolSequenceRandomSize {
		_, err := io.ReadFull(rander, poolSequenceRandom[:])
		if err != nil {
			s.Unlock()
			panic(err)
		}
		poolSequenceRandomPos = 0
	}
	copy(buf[:5], Uint64ToBytes(uint64(now-twepoch) << workerInfoBits)[:5])
	copy(buf[5:8], s.ip)
	// 随机数
	copy(buf[8:14], poolSequenceRandom[poolSequenceRandomPos:(poolSequenceRandomPos+6)])
	poolSequenceRandomPos += 6
	// sequence
	copy(buf[14:], Uint64ToBytes(uint64(s.sequence)))
	s.Unlock()
	return base64.RawURLEncoding.EncodeToString(buf)
}

func Uint64ToBytes(n uint64) []byte {
	bytesBuffer := make([]byte, 8)
	binary.BigEndian.PutUint64(bytesBuffer, n)
	return bytesBuffer
}

func BytesToUint64(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}

func (s *Config) GetTime(encode string) string {
	decoded, _ := base64.RawURLEncoding.DecodeString(encode)
	tm := time.Unix(int64(BytesToUint64(decoded[:8])>>workerInfoBits+twepoch)/1000, 0)
	return tm.Format("2006-01-02 15:04:05") // 2018-7-15 15:23:00
}

func (s *Config) GetIP(encode string) string {
	decoded, _ := base64.RawURLEncoding.DecodeString(encode)
	decoded[5] = decoded[5] ^ s.Mask1
	decoded[6] = decoded[6] ^ s.Mask2
	decoded[7] = decoded[7] ^ s.Mask3
	return fmt.Sprintf("xxx.%v.%v.%v", decoded[5], decoded[6], decoded[7])
}
