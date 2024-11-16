package esnowflake

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfigWithValidIP_ReturnsConfig(t *testing.T) {
	config := New("192.168.1.1", 1, 2, 3)
	if config == nil {
		t.Error("Expected non-nil config")
	}
}

func TestConfigWithInvalidIP_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for invalid IP")
		}
	}()
	New("invalid_ip", 1, 2, 3)
}

func TestConfigWithValidIP_SetsMaskedIP(t *testing.T) {
	config := New("192.168.1.2", 1, 2, 3)
	expectedIP := []byte{168 ^ 1, 1 ^ 2, 2 ^ 3}
	for i, b := range expectedIP {
		if config.ip[i] != b {
			t.Errorf("Expected masked IP byte %d to be %d, got %d", i, b, config.ip[i])
		}
	}
}

func TestConfigWithValidIP_GetIp(t *testing.T) {
	config := New("192.168.1.2", 1, 2, 3)
	encode := config.GenerateByRandom()
	ip := config.GetIP(encode)
	assert.Equal(t, "xxx.168.1.2", ip)

}

func TestGenerateByRandom_ReturnsUniqueIDs(t *testing.T) {
	config := New("192.168.1.1", 1, 2, 3)
	id1 := config.GenerateByRandom()
	id2 := config.GenerateByRandom()
	if id1 == id2 {
		t.Errorf("Expected unique IDs, but got %s and %s", id1, id2)
	}
}

func TestGenerateByRandom_HandlesPoolRefill(t *testing.T) {
	config := New("192.168.1.1", 1, 2, 3)
	for i := 0; i < 100; i++ {
		config.GenerateByRandom()
	}
}

func TestGenerateByRandom_ConcurrencyNotEqual(t *testing.T) {
	config := New("192.168.1.1", 1, 2, 3)
	testLength := 1000000
	output := sync.Map{}
	// 测试并发生成随机数，是否重复
	wg := sync.WaitGroup{}
	wg.Add(testLength)
	for i := 0; i < testLength; i++ {
		go func() {
			id := config.GenerateByRandom()
			_, flag := output.Load(id)
			if flag {
				t.Errorf("Expected unique IDs, but got %s and %s", id, id)
			}
			output.Store(id, struct{}{})
			wg.Done()
		}()

	}
	wg.Wait()
}

func TestGenerateBySequence_ConcurrencyNotEqual(t *testing.T) {
	config := New("192.168.1.1", 1, 2, 3)
	testLength := 1000000
	output := sync.Map{}
	// 测试并发生成随机数，是否重复
	wg := sync.WaitGroup{}
	wg.Add(testLength)
	for i := 0; i < testLength; i++ {
		go func() {
			id := config.GenerateBySequence()
			_, flag := output.Load(id)
			if flag {
				t.Errorf("Expected unique IDs, but got %s and %s", id, id)
			}
			output.Store(id, struct{}{})
			wg.Done()
		}()

	}
	wg.Wait()
}

func TestGenerateByRandom_GetTime(t *testing.T) {
	config := New("192.168.1.1", 1, 2, 3)
	encode := config.GenerateByRandom()
	encodeTime := config.GetTime(encode)
	fmt.Printf("time--------------->"+"%+v\n", encodeTime)
}

func TestGenerateByRandom_HandlesLocking(t *testing.T) {
	config := New("192.168.1.1", 1, 2, 3)
	done := make(chan bool)
	go func() {
		config.GenerateByRandom()
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("GenerateByRandom did not return within 1 second")
	}
}

func TestGenerateBySequence_ReturnsUniqueIDs(t *testing.T) {
	config := New("192.168.1.1", 1, 2, 3)
	id1 := config.GenerateBySequence()
	id2 := config.GenerateBySequence()
	if id1 == id2 {
		t.Errorf("Expected unique IDs, but got %s and %s", id1, id2)
	}
}

func TestGenerateBySequence_HandlesSequenceOverflow(t *testing.T) {
	config := New("192.168.1.1", 1, 2, 3)
	config.timestamp = time.Now().UnixNano() / 1000000
	config.sequence = sequenceMask
	id := config.GenerateBySequence()
	if id == "" {
		t.Error("Expected non-empty ID on sequence overflow")
	}
}

func TestGenerateBySequence_HandlesLocking(t *testing.T) {
	config := New("192.168.1.1", 1, 2, 3)
	done := make(chan bool)
	go func() {
		config.GenerateBySequence()
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("GenerateBySequence did not return within 1 second")
	}
}

func BenchmarkRandomAndSequence(b *testing.B) {
	obj := New("192.168.1.1", 1, 2, 3)
	b.Run("Random", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			obj.GenerateByRandom()
		}
	})
	b.Run("Sequence", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			obj.GenerateBySequence()
		}
	})
}
