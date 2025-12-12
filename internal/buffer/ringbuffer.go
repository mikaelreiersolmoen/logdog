package buffer

import "sync"

// RingBuffer is a thread-safe circular buffer for log entries
type RingBuffer struct {
	entries  []string
	capacity int
	start    int // Index of oldest entry
	size     int // Current number of entries
	mu       sync.RWMutex
}

// NewRingBuffer creates a new circular buffer with the specified capacity
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		entries:  make([]string, capacity),
		capacity: capacity,
		start:    0,
		size:     0,
	}
}

// Add appends a new entry to the buffer
func (rb *RingBuffer) Add(entry string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.size < rb.capacity {
		// Buffer not yet full
		rb.entries[rb.size] = entry
		rb.size++
	} else {
		// Buffer full, overwrite oldest entry
		rb.entries[rb.start] = entry
		rb.start = (rb.start + 1) % rb.capacity
	}
}

// AddBatch appends multiple entries efficiently
func (rb *RingBuffer) AddBatch(entries []string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for _, entry := range entries {
		if rb.size < rb.capacity {
			rb.entries[rb.size] = entry
			rb.size++
		} else {
			rb.entries[rb.start] = entry
			rb.start = (rb.start + 1) % rb.capacity
		}
	}
}

// Get returns entries in chronological order (oldest to newest)
func (rb *RingBuffer) Get() []string {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.size == 0 {
		return []string{}
	}

	result := make([]string, rb.size)
	if rb.size < rb.capacity {
		// Buffer not yet wrapped
		copy(result, rb.entries[:rb.size])
	} else {
		// Buffer wrapped, need to reconstruct order
		copy(result, rb.entries[rb.start:])
		copy(result[rb.capacity-rb.start:], rb.entries[:rb.start])
	}
	return result
}

// Size returns the current number of entries
func (rb *RingBuffer) Size() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.size
}

// Clear removes all entries
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.size = 0
	rb.start = 0
}
