package storage

/*
#cgo CFLAGS: -I.
#cgo LDFLAGS: -L. -lvector_search
#include "vector_search.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// VectorStore is the interface for a vector store.
type VectorStore interface {
	Add(vectors [][]float32, documents []string) error
	Query(vector []float32, k int) ([]string, error)
	Close() error
}

// NewVectorStore creates a new vector store.
func NewVectorStore(dim int) (VectorStore, error) {
	return &CGoStore{
		dim: dim,
	}, nil
}

// CGoStore implements the VectorStore interface using CGo.
type CGoStore struct {
	index *C.VectorIndex
	docs  []string
	dim   int

	// Pointers to C-allocated memory that must be manually freed in Close().
	cVectors *C.Vector
	cData    unsafe.Pointer
}

// Add adds vectors and their corresponding documents to the store.
// It allocates memory on the C heap to avoid passing Go pointers to C.
func (s *CGoStore) Add(vectors [][]float32, documents []string) error {
	// If a previous index exists, free its memory before creating a new one.
	if s.index != nil {
		s.Close()
	}

	s.docs = documents
	numVectors := len(vectors)
	if numVectors == 0 {
		s.index = nil
		return nil
	}
	dim := len(vectors[0])

	floatSize := unsafe.Sizeof(float32(0))
	vectorStructSize := unsafe.Sizeof(C.Vector{})

	// Allocate one contiguous block on the C heap for all vector float data.
	totalDataSize := numVectors * dim * int(floatSize)
	cData := C.malloc(C.size_t(totalDataSize))
	if cData == nil {
		return fmt.Errorf("failed to allocate memory for vector data")
	}

	// Allocate memory on the C heap for the array of C.Vector structs.
	cVectorsArraySize := numVectors * int(vectorStructSize)
	cVectors := (*C.Vector)(C.malloc(C.size_t(cVectorsArraySize)))
	if cVectors == nil {
		C.free(cData) // Clean up previous allocation
		return fmt.Errorf("failed to allocate memory for vector structs")
	}

	// Store pointers in the struct so they can be freed later in Close().
	s.cData = cData
	s.cVectors = cVectors

	// Create a Go slice header for the C-allocated vector structs to allow easy access.
	cVectorsSlice := (*[1 << 30]C.Vector)(unsafe.Pointer(cVectors))[:numVectors:numVectors]

	var currentOffset uintptr
	for i, v := range vectors {
		destination := unsafe.Pointer(uintptr(cData) + currentOffset)
		bytesToCopy := len(v) * int(floatSize)
		C.memcpy(destination, unsafe.Pointer(&v[0]), C.size_t(bytesToCopy))
		cVectorsSlice[i].data = (*C.float)(destination)
		cVectorsSlice[i].len = C.int(len(v))
		currentOffset += uintptr(bytesToCopy)
	}

	s.index = C.create_index(cVectors, C.int(numVectors))
	return nil
}

// Query queries the store for the k most similar documents.
func (s *CGoStore) Query(vector []float32, k int) ([]string, error) {
	if s.index == nil {
		return nil, fmt.Errorf("index is not initialized")
	}

	floatSize := unsafe.Sizeof(float32(0))
	queryDataSize := len(vector) * int(floatSize)

	// Allocate temporary C memory for the query vector.
	cQueryData := C.malloc(C.size_t(queryDataSize))
	if cQueryData == nil {
		return nil, fmt.Errorf("failed to allocate memory for query vector")
	}
	defer C.free(cQueryData)

	C.memcpy(cQueryData, unsafe.Pointer(&vector[0]), C.size_t(queryDataSize))

	cQuery := C.Vector{
		data: (*C.float)(cQueryData),
		len:  C.int(len(vector)),
	}

	cNeighbors := C.knn_search(s.index, &cQuery, C.int(k))
	defer C.free(unsafe.Pointer(cNeighbors))

	neighbors := (*[1 << 30]C.int)(unsafe.Pointer(cNeighbors))[:k:k]
	results := make([]string, k)
	for i := 0; i < k; i++ {
		results[i] = s.docs[neighbors[i]]
	}
	return results, nil
}

// Close frees all C-allocated memory associated with the CGoStore.
func (s *CGoStore) Close() error {
	if s.index != nil {
		C.free_index(s.index)
		s.index = nil
	}
	if s.cVectors != nil {
		C.free(unsafe.Pointer(s.cVectors))
		s.cVectors = nil
	}
	if s.cData != nil {
		C.free(s.cData)
		s.cData = nil
	}
	return nil
}
