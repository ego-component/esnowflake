/*
* 1                                             41                           65                                                     113                128
* +---------------------------------------------+----------------------------+------------------------------------------------------+-------------------+
* | timestamp(ms)                               | worker info                | random number                                        | sequence          |
* +---------------------------------------------+----------------------------+------------------------------------------------------+-------------------+
* | 0000000000 0000000000 0000000000 0000000000 | 0000000000 0000000000 0000 | 0000000000 0000000000 0000000000 0000000000 00000000 | 00000000 00000000 |
* +---------------------------------------------+----------------------------+------------------------------------------------------+-------------------+
*
* 1. 40 位时间截(毫秒级)，注意这是时间截的差值（当前时间截 - 开始时间截)。可以使用约 34 年: (1L << 40) / (1000L * 60 * 60 * 24 * 365) = 34。（2020-2054）
* 2. 24 位 worker info 数据，适应 k8s 环境。
* 3. 64 随机数
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

const randPoolSequenceSize = 8 * 6
const randPoolRandomSize = 8 * 6

var (
	rander          = rand.Reader              // random function
	poolSequencePos = randPoolSequenceSize     // protected with poolMu
	poolSequence    [randPoolSequenceSize]byte // protected with poolMu
	poolRandomPos   = randPoolRandomSize       // protected with poolMu
	poolRandom      [randPoolRandomSize]byte   // protected with poolMu
	//mask         = [3]byte{123, 45, 67}
	sequenceBits = uint(16)                         // 序列所占的位数
	sequenceMask = int64(-1 ^ (-1 << sequenceBits)) //

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

func (s *Config) GenerateByRandom() string {
	buf := make([]byte, 16)
	s.Lock()
	now := time.Now().UnixNano() / 1000000
	if poolRandomPos == randPoolRandomSize {
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

	if poolSequencePos == randPoolSequenceSize {
		_, err := io.ReadFull(rander, poolSequence[:])
		if err != nil {
			s.Unlock()
			panic(err)
		}
		poolSequencePos = 0
	}
	copy(buf[:5], Uint64ToBytes(uint64(now-twepoch) << workerInfoBits)[:5])
	copy(buf[5:8], s.ip)
	copy(buf[8:14], poolSequence[poolSequencePos:(poolSequencePos+7)])
	poolSequencePos += 7
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
