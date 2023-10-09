package counter

import (
	"fmt"
	"sync/atomic"

	"github.com/inhies/go-bytesize"
)

// totalReadBytes is the total number of bytes read
var _totalReadBytes uint64 = 0

// totalWrittenBytes is the total number of bytes written
var _totalWrittenBytes uint64 = 0

var _totalReadBytesRelay uint64 = 0

var _totalWrittenBytesRelay uint64 = 0


// IncrReadBytesRelay increments the number of bytes read
func IncrReadBytesRelay(n int) {
	atomic.AddUint64(&_totalReadBytesRelay, uint64(n))
}

// IncrWrittenBytesRelay increments the number of bytes written
func IncrWrittenBytesRelay(n int) {
	atomic.AddUint64(&_totalWrittenBytesRelay, uint64(n))
}

// GetReadBytesRelay returns the number of bytes read
func GetReadBytesRelay() uint64 {
	return atomic.LoadUint64(&_totalReadBytesRelay)
}

// GetWrittenBytesRelay returns the number of bytes written
func GetWrittenBytesRelay() uint64 {
	return atomic.LoadUint64(&_totalWrittenBytesRelay)
}



// IncrReadBytes increments the number of bytes read
func IncrReadBytes(n int) {
	atomic.AddUint64(&_totalReadBytes, uint64(n))
}

// IncrWrittenBytes increments the number of bytes written
func IncrWrittenBytes(n int) {
	atomic.AddUint64(&_totalWrittenBytes, uint64(n))
}

// GetReadBytes returns the number of bytes read
func GetReadBytes() uint64 {
	return atomic.LoadUint64(&_totalReadBytes)
}

// GetWrittenBytes returns the number of bytes written
func GetWrittenBytes() uint64 {
	return atomic.LoadUint64(&_totalWrittenBytes)
}

// PrintBytes returns the bytes info
func PrintBytes(serverMode bool) string {
	if serverMode {
		return fmt.Sprintf("download %v upload %v", bytesize.New(float64(GetWrittenBytes())).String(), bytesize.New(float64(GetReadBytes())).String())
	}
	return fmt.Sprintf("Client: download %v upload %v Relayed: download %v upload %v", 
		bytesize.New(float64(GetReadBytes())).String(), 
		bytesize.New(float64(GetWrittenBytes())).String(),
		bytesize.New(float64(GetReadBytesRelay())).String(),
		bytesize.New(float64(GetWrittenBytesRelay())).String(),
	)
}
