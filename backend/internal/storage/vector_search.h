#ifndef VECTOR_SEARCH_H
#define VECTOR_SEARCH_H

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    float* data;
    int len;
} Vector;

// Priority queue element for search candidates
typedef struct {
    int node_id;
    float distance;
} SearchCandidate;

// HNSW node representing a single vector in the graph
typedef struct {
    int vector_id;                    // Index into the original vectors array
    int maximum_layer;                // Highest layer this node exists in
    int** layer_connections;          // Array of connection arrays for each layer
    int* connection_counts;           // Number of connections at each layer
    int* allocated_connection_sizes;  // Allocated space for connections at each layer
} HNSWNode;

// HNSW graph structure for efficient vector search
typedef struct {
    HNSWNode* nodes;                  // Array of all nodes in the graph
    Vector* original_vectors;         // Reference to original vector data
    int node_count;                   // Total number of nodes
    int entry_point_node_id;          // Entry point node ID for search
    int maximum_layer_in_graph;       // Highest layer in the entire graph
    
    // HNSW hyperparameters
    int max_connections_per_node;     // M: max connections per node (except layer 0)
    int max_connections_layer_zero;   // Mmax: max connections at layer 0
    float level_generation_factor;    // ml: level generation factor
    int construction_search_width;    // efConstruction: candidate list size during construction
} HNSWGraph;

// Enhanced vector index supporting both brute-force and HNSW search
typedef struct {
    Vector* vectors;
    int len;
    HNSWGraph* hnsw_graph;           // Optional HNSW graph for fast search
    int use_hnsw_optimization;       // Flag to enable HNSW search
} VectorIndex;

// Search configuration for optimized searches
typedef struct {
    int search_width;                // ef: dynamic candidate list size
    int max_distance_computations;   // Early termination limit
    float accuracy_threshold;        // Stop when this accuracy is reached
    int use_approximate_search;      // Enable approximate search mode
} SearchConfig;

// Traditional API (maintains backward compatibility)
VectorIndex* create_index(Vector* vectors, int len);
int* knn_search(VectorIndex* index, Vector* query, int k);
void free_index(VectorIndex* index);

// Enhanced HNSW API
VectorIndex* create_hnsw_index(Vector* vectors, int len, int max_connections, 
                              int max_connections_layer_zero, float level_factor);
HNSWGraph* build_hnsw_graph(Vector* vectors, int vector_count, int max_connections,
                           int max_connections_layer_zero, float level_factor, 
                           int construction_search_width);

// Optimized search functions
int* hnsw_knn_search(VectorIndex* index, Vector* query, int k, SearchConfig* config);
int* approximate_search(VectorIndex* index, Vector* query, int k, int search_width);
int* beam_search(VectorIndex* index, Vector* query, int k, int beam_width);

// Utility functions
float calculate_euclidean_distance(Vector* vector_a, Vector* vector_b);
int determine_random_layer(float level_generation_factor);
void free_hnsw_graph(HNSWGraph* graph);

#ifdef __cplusplus
}
#endif

#endif // VECTOR_SEARCH_H
