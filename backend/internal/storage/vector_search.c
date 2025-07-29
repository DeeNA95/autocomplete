#include "vector_search.h"
#include <stdlib.h>
#include <math.h>
#include <float.h>
#include <string.h>
#include <time.h>
#include <limits.h>

// ================================
// UTILITY FUNCTIONS
// ================================

float calculate_euclidean_distance(Vector* vector_a, Vector* vector_b) {
    if (vector_a->len != vector_b->len) {
        return FLT_MAX; // Invalid comparison
    }
    
    float distance_squared = 0.0f;
    for (int dimension_index = 0; dimension_index < vector_a->len; dimension_index++) {
        float dimension_difference = vector_a->data[dimension_index] - vector_b->data[dimension_index];
        distance_squared += dimension_difference * dimension_difference;
    }
    return sqrtf(distance_squared);
}

int determine_random_layer(float level_generation_factor) {
    static int random_seed_initialized = 0;
    if (!random_seed_initialized) {
        srand((unsigned int)time(NULL));
        random_seed_initialized = 1;
    }
    
    int layer = 0;
    while (((float)rand() / RAND_MAX) < level_generation_factor) {
        layer++;
    }
    return layer;
}

// ================================
// PRIORITY QUEUE FOR SEARCH CANDIDATES
// ================================

typedef struct {
    SearchCandidate* candidates;
    int size;
    int capacity;
    int is_max_heap; // 1 for max-heap, 0 for min-heap
} PriorityQueue;

PriorityQueue* create_priority_queue(int capacity, int is_max_heap) {
    PriorityQueue* queue = (PriorityQueue*)malloc(sizeof(PriorityQueue));
    queue->candidates = (SearchCandidate*)malloc(sizeof(SearchCandidate) * capacity);
    queue->size = 0;
    queue->capacity = capacity;
    queue->is_max_heap = is_max_heap;
    return queue;
}

void swap_candidates(SearchCandidate* candidate_a, SearchCandidate* candidate_b) {
    SearchCandidate temp = *candidate_a;
    *candidate_a = *candidate_b;
    *candidate_b = temp;
}

void heapify_up(PriorityQueue* queue, int child_index) {
    int parent_index = (child_index - 1) / 2;
    if (parent_index < 0) return;
    
    int should_swap = queue->is_max_heap ? 
        (queue->candidates[child_index].distance > queue->candidates[parent_index].distance) :
        (queue->candidates[child_index].distance < queue->candidates[parent_index].distance);
    
    if (should_swap) {
        swap_candidates(&queue->candidates[child_index], &queue->candidates[parent_index]);
        heapify_up(queue, parent_index);
    }
}

void heapify_down(PriorityQueue* queue, int parent_index) {
    int left_child = 2 * parent_index + 1;
    int right_child = 2 * parent_index + 2;
    int target_index = parent_index;
    
    if (left_child < queue->size) {
        int left_is_better = queue->is_max_heap ?
            (queue->candidates[left_child].distance > queue->candidates[target_index].distance) :
            (queue->candidates[left_child].distance < queue->candidates[target_index].distance);
        if (left_is_better) target_index = left_child;
    }
    
    if (right_child < queue->size) {
        int right_is_better = queue->is_max_heap ?
            (queue->candidates[right_child].distance > queue->candidates[target_index].distance) :
            (queue->candidates[right_child].distance < queue->candidates[target_index].distance);
        if (right_is_better) target_index = right_child;
    }
    
    if (target_index != parent_index) {
        swap_candidates(&queue->candidates[parent_index], &queue->candidates[target_index]);
        heapify_down(queue, target_index);
    }
}

void insert_candidate(PriorityQueue* queue, int node_id, float distance) {
    if (queue->size < queue->capacity) {
        queue->candidates[queue->size].node_id = node_id;
        queue->candidates[queue->size].distance = distance;
        heapify_up(queue, queue->size);
        queue->size++;
    } else {
        // For max-heap (worst candidates), replace if new candidate is better
        if (queue->is_max_heap && distance < queue->candidates[0].distance) {
            queue->candidates[0].node_id = node_id;
            queue->candidates[0].distance = distance;
            heapify_down(queue, 0);
        }
        // For min-heap (best candidates), replace if new candidate is worse
        else if (!queue->is_max_heap && distance > queue->candidates[0].distance) {
            queue->candidates[0].node_id = node_id;
            queue->candidates[0].distance = distance;
            heapify_down(queue, 0);
        }
    }
}

SearchCandidate extract_top_candidate(PriorityQueue* queue) {
    SearchCandidate top = queue->candidates[0];
    queue->size--;
    if (queue->size > 0) {
        queue->candidates[0] = queue->candidates[queue->size];
        heapify_down(queue, 0);
    }
    return top;
}

void free_priority_queue(PriorityQueue* queue) {
    free(queue->candidates);
    free(queue);
}

// ================================
// HNSW NODE MANAGEMENT
// ================================

HNSWNode* create_hnsw_node(int vector_id, int maximum_layer) {
    HNSWNode* node = (HNSWNode*)malloc(sizeof(HNSWNode));
    node->vector_id = vector_id;
    node->maximum_layer = maximum_layer;
    
    // Allocate connection arrays for each layer (0 to maximum_layer)
    node->layer_connections = (int**)malloc(sizeof(int*) * (maximum_layer + 1));
    node->connection_counts = (int*)calloc(maximum_layer + 1, sizeof(int));
    node->allocated_connection_sizes = (int*)malloc(sizeof(int) * (maximum_layer + 1));
    
    // Initialize connection arrays
    for (int layer = 0; layer <= maximum_layer; layer++) {
        int initial_capacity = (layer == 0) ? 32 : 16; // More connections expected at layer 0
        node->layer_connections[layer] = (int*)malloc(sizeof(int) * initial_capacity);
        node->allocated_connection_sizes[layer] = initial_capacity;
    }
    
    return node;
}

void add_connection_to_node(HNSWNode* node, int layer, int connected_node_id) {
    if (layer > node->maximum_layer) return;
    
    // Check if connection already exists
    for (int connection_index = 0; connection_index < node->connection_counts[layer]; connection_index++) {
        if (node->layer_connections[layer][connection_index] == connected_node_id) {
            return; // Connection already exists
        }
    }
    
    // Expand array if needed
    if (node->connection_counts[layer] >= node->allocated_connection_sizes[layer]) {
        node->allocated_connection_sizes[layer] *= 2;
        node->layer_connections[layer] = (int*)realloc(
            node->layer_connections[layer],
            sizeof(int) * node->allocated_connection_sizes[layer]
        );
    }
    
    node->layer_connections[layer][node->connection_counts[layer]] = connected_node_id;
    node->connection_counts[layer]++;
}

void free_hnsw_node(HNSWNode* node) {
    for (int layer = 0; layer <= node->maximum_layer; layer++) {
        free(node->layer_connections[layer]);
    }
    free(node->layer_connections);
    free(node->connection_counts);
    free(node->allocated_connection_sizes);
    free(node);
}

// ================================
// HNSW GRAPH CONSTRUCTION
// ================================

HNSWGraph* build_hnsw_graph(Vector* vectors, int vector_count, int max_connections,
                           int max_connections_layer_zero, float level_factor, 
                           int construction_search_width) {
    HNSWGraph* graph = (HNSWGraph*)malloc(sizeof(HNSWGraph));
    graph->nodes = (HNSWNode*)malloc(sizeof(HNSWNode) * vector_count);
    graph->original_vectors = vectors;
    graph->node_count = vector_count;
    graph->entry_point_node_id = 0;
    graph->maximum_layer_in_graph = 0;
    graph->max_connections_per_node = max_connections;
    graph->max_connections_layer_zero = max_connections_layer_zero;
    graph->level_generation_factor = level_factor;
    graph->construction_search_width = construction_search_width;
    
    // Initialize all nodes first
    for (int vector_index = 0; vector_index < vector_count; vector_index++) {
        int node_layer = determine_random_layer(level_factor);
        graph->nodes[vector_index] = *create_hnsw_node(vector_index, node_layer);
        
        if (node_layer > graph->maximum_layer_in_graph) {
            graph->maximum_layer_in_graph = node_layer;
            graph->entry_point_node_id = vector_index;
        }
    }
    
    // Build connections by inserting each node
    for (int current_node_id = 1; current_node_id < vector_count; current_node_id++) {
        HNSWNode* current_node = &graph->nodes[current_node_id];
        Vector* current_vector = &vectors[current_node_id];
        
        // Search for closest nodes at each layer
        PriorityQueue* closest_candidates = create_priority_queue(construction_search_width, 0); // min-heap
        
        // Start from entry point and search down to layer 0
        int current_search_node = graph->entry_point_node_id;
        
        // Greedy search from top layer down to target layer + 1
        for (int search_layer = graph->maximum_layer_in_graph; 
             search_layer > current_node->maximum_layer; search_layer--) {
            
            float best_distance = calculate_euclidean_distance(
                current_vector, &vectors[current_search_node]
            );
            
            // Find closest node at this layer
            HNSWNode* search_node = &graph->nodes[current_search_node];
            if (search_layer <= search_node->maximum_layer) {
                for (int connection_index = 0; 
                     connection_index < search_node->connection_counts[search_layer]; 
                     connection_index++) {
                    
                    int neighbor_id = search_node->layer_connections[search_layer][connection_index];
                    float neighbor_distance = calculate_euclidean_distance(
                        current_vector, &vectors[neighbor_id]
                    );
                    
                    if (neighbor_distance < best_distance) {
                        best_distance = neighbor_distance;
                        current_search_node = neighbor_id;
                    }
                }
            }
        }
        
        // Search and connect at layers from maximum_layer down to 0
        for (int connection_layer = current_node->maximum_layer; 
             connection_layer >= 0; connection_layer--) {
            
            // Beam search at current layer
            PriorityQueue* layer_candidates = create_priority_queue(construction_search_width, 0);
            PriorityQueue* visited_nodes = create_priority_queue(construction_search_width * 2, 1); // max-heap
            
            insert_candidate(layer_candidates, current_search_node, 
                           calculate_euclidean_distance(current_vector, &vectors[current_search_node]));
            insert_candidate(visited_nodes, current_search_node, 
                           calculate_euclidean_distance(current_vector, &vectors[current_search_node]));
            
            while (layer_candidates->size > 0) {
                SearchCandidate current_candidate = extract_top_candidate(layer_candidates);
                
                // Explore neighbors
                HNSWNode* candidate_node = &graph->nodes[current_candidate.node_id];
                if (connection_layer <= candidate_node->maximum_layer) {
                    for (int neighbor_index = 0; 
                         neighbor_index < candidate_node->connection_counts[connection_layer]; 
                         neighbor_index++) {
                        
                        int neighbor_id = candidate_node->layer_connections[connection_layer][neighbor_index];
                        float neighbor_distance = calculate_euclidean_distance(
                            current_vector, &vectors[neighbor_id]
                        );
                        
                        // Check if this neighbor improves our candidate set
                        if (visited_nodes->size < construction_search_width || 
                            neighbor_distance < visited_nodes->candidates[0].distance) {
                            
                            insert_candidate(layer_candidates, neighbor_id, neighbor_distance);
                            insert_candidate(visited_nodes, neighbor_id, neighbor_distance);
                        }
                    }
                }
            }
            
            // Select best connections for this layer using diverse candidate selection
            int max_connections_this_layer = (connection_layer == 0) ? 
                max_connections_layer_zero : max_connections;
            
            // Convert visited nodes to array for better selection algorithm
            SearchCandidate* candidates_array = (SearchCandidate*)malloc(sizeof(SearchCandidate) * visited_nodes->size);
            int candidate_count = 0;
            while (visited_nodes->size > 0) {
                candidates_array[candidate_count++] = extract_top_candidate(visited_nodes);
            }
            
            // Select diverse connections to avoid clustering
            int connections_to_make = (candidate_count < max_connections_this_layer) ? 
                candidate_count : max_connections_this_layer;
            
            int* selected_connections = (int*)malloc(sizeof(int) * connections_to_make);
            int selected_count = 0;
            
            // Always include the closest candidate
            if (candidate_count > 0 && candidates_array[0].node_id != current_node_id) {
                selected_connections[selected_count++] = candidates_array[0].node_id;
            }
            
            // Select remaining candidates with diversity consideration
            for (int candidate_index = 1; candidate_index < candidate_count && selected_count < connections_to_make; candidate_index++) {
                if (candidates_array[candidate_index].node_id == current_node_id) continue;
                
                int should_select = 1;
                Vector* candidate_vector = &graph->original_vectors[candidates_array[candidate_index].node_id];
                
                // Check diversity with already selected connections
                for (int selected_index = 0; selected_index < selected_count; selected_index++) {
                    Vector* selected_vector = &graph->original_vectors[selected_connections[selected_index]];
                    float similarity = calculate_euclidean_distance(candidate_vector, selected_vector);
                    
                    // Reject if too similar to already selected (clustering prevention)
                    if (similarity < candidates_array[candidate_index].distance * 0.7f) {
                        should_select = 0;
                        break;
                    }
                }
                
                if (should_select) {
                    selected_connections[selected_count++] = candidates_array[candidate_index].node_id;
                }
            }
            
            // If we still need more connections, add remaining candidates
            for (int candidate_index = 1; candidate_index < candidate_count && selected_count < connections_to_make; candidate_index++) {
                if (candidates_array[candidate_index].node_id == current_node_id) continue;
                
                int already_selected = 0;
                for (int selected_index = 0; selected_index < selected_count; selected_index++) {
                    if (selected_connections[selected_index] == candidates_array[candidate_index].node_id) {
                        already_selected = 1;
                        break;
                    }
                }
                
                if (!already_selected) {
                    selected_connections[selected_count++] = candidates_array[candidate_index].node_id;
                }
            }
            
            // Make bidirectional connections
            for (int connection_index = 0; connection_index < selected_count; connection_index++) {
                add_connection_to_node(current_node, connection_layer, selected_connections[connection_index]);
                add_connection_to_node(&graph->nodes[selected_connections[connection_index]], 
                                     connection_layer, current_node_id);
            }
            
            free(candidates_array);
            free(selected_connections);
            
            free_priority_queue(layer_candidates);
            free_priority_queue(visited_nodes);
        }
        
        free_priority_queue(closest_candidates);
    }
    
    return graph;
}

// ================================
// SEARCH ALGORITHMS
// ================================

int* search_layer(HNSWGraph* graph, Vector* query, int entry_point, int layer, 
                  int search_width, int* result_count) {
    PriorityQueue* candidates = create_priority_queue(search_width, 0); // min-heap for closest
    PriorityQueue* visited = create_priority_queue(search_width * 2, 1); // max-heap for worst
    int* visited_flags = (int*)calloc(graph->node_count, sizeof(int));
    
    float entry_distance = calculate_euclidean_distance(query, &graph->original_vectors[entry_point]);
    insert_candidate(candidates, entry_point, entry_distance);
    insert_candidate(visited, entry_point, entry_distance);
    visited_flags[entry_point] = 1;
    
    while (candidates->size > 0) {
        SearchCandidate current = extract_top_candidate(candidates);
        
        // Early termination if current distance is worse than worst in visited set
        if (visited->size >= search_width && current.distance > visited->candidates[0].distance) {
            break;
        }
        
        // Explore neighbors
        HNSWNode* current_node = &graph->nodes[current.node_id];
        if (layer <= current_node->maximum_layer) {
            for (int neighbor_index = 0; 
                 neighbor_index < current_node->connection_counts[layer]; 
                 neighbor_index++) {
                
                int neighbor_id = current_node->layer_connections[layer][neighbor_index];
                
                if (!visited_flags[neighbor_id]) {
                    visited_flags[neighbor_id] = 1;
                    float neighbor_distance = calculate_euclidean_distance(
                        query, &graph->original_vectors[neighbor_id]
                    );
                    
                    if (visited->size < search_width || 
                        neighbor_distance < visited->candidates[0].distance) {
                        
                        insert_candidate(candidates, neighbor_id, neighbor_distance);
                        insert_candidate(visited, neighbor_id, neighbor_distance);
                    }
                }
            }
        }
    }
    
    // Extract results
    *result_count = visited->size;
    int* results = (int*)malloc(sizeof(int) * (*result_count));
    
    // Convert max-heap to sorted array (closest first)
    for (int result_index = *result_count - 1; result_index >= 0; result_index--) {
        SearchCandidate result = extract_top_candidate(visited);
        results[result_index] = result.node_id;
    }
    
    free_priority_queue(candidates);
    free_priority_queue(visited);
    free(visited_flags);
    
    return results;
}

int* hnsw_knn_search(VectorIndex* index, Vector* query, int k, SearchConfig* search_config) {
    if (!index->hnsw_graph) {
        return NULL; // No HNSW graph available
    }
    
    HNSWGraph* graph = index->hnsw_graph;
    int search_width = search_config ? search_config->search_width : k * 2;
    
    // Start from entry point and search down through layers
    int current_closest = graph->entry_point_node_id;
    
    // Greedy search from top layer down to layer 1
    for (int layer = graph->maximum_layer_in_graph; layer > 0; layer--) {
        int result_count;
        int* layer_results = search_layer(graph, query, current_closest, layer, 1, &result_count);
        if (result_count > 0) {
            current_closest = layer_results[0];
        }
        free(layer_results);
    }
    
    // Comprehensive search at layer 0
    int result_count;
    int* all_candidates = search_layer(graph, query, current_closest, 0, search_width, &result_count);
    
    // Return top k results
    int return_count = (result_count < k) ? result_count : k;
    int* final_results = (int*)malloc(sizeof(int) * return_count);
    memcpy(final_results, all_candidates, sizeof(int) * return_count);
    
    free(all_candidates);
    return final_results;
}

int* approximate_search(VectorIndex* index, Vector* query, int k, int search_width) {
    SearchConfig config = {
        .search_width = search_width,
        .max_distance_computations = search_width * 10,
        .accuracy_threshold = 0.9f,
        .use_approximate_search = 1
    };
    return hnsw_knn_search(index, query, k, &config);
}

int* beam_search(VectorIndex* index, Vector* query, int k, int beam_width) {
    SearchConfig config = {
        .search_width = beam_width,
        .max_distance_computations = beam_width * 5,
        .accuracy_threshold = 0.95f,
        .use_approximate_search = 0
    };
    return hnsw_knn_search(index, query, k, &config);
}

// ================================
// TRADITIONAL BRUTE-FORCE SEARCH
// ================================

int* knn_search(VectorIndex* index, Vector* query, int k) {
    // Use HNSW if available
    if (index->use_hnsw_optimization && index->hnsw_graph) {
        SearchConfig default_config = {
            .search_width = k * 4,
            .max_distance_computations = INT_MAX,
            .accuracy_threshold = 1.0f,
            .use_approximate_search = 0
        };
        return hnsw_knn_search(index, query, k, &default_config);
    }
    
    // Fallback to brute-force search
    int* neighbors = (int*)malloc(sizeof(int) * k);
    float* distances = (float*)malloc(sizeof(float) * k);

    for (int neighbor_index = 0; neighbor_index < k; neighbor_index++) {
        neighbors[neighbor_index] = -1;
        distances[neighbor_index] = FLT_MAX;
    }

    for (int vector_index = 0; vector_index < index->len; vector_index++) {
        float current_distance = calculate_euclidean_distance(query, &index->vectors[vector_index]);

        for (int insertion_position = 0; insertion_position < k; insertion_position++) {
            if (current_distance < distances[insertion_position]) {
                // Shift elements to make room
                for (int shift_index = k - 1; shift_index > insertion_position; shift_index--) {
                    distances[shift_index] = distances[shift_index - 1];
                    neighbors[shift_index] = neighbors[shift_index - 1];
                }
                distances[insertion_position] = current_distance;
                neighbors[insertion_position] = vector_index;
                break;
            }
        }
    }

    free(distances);
    return neighbors;
}

// ================================
// INDEX CREATION AND MANAGEMENT
// ================================

VectorIndex* create_index(Vector* vectors, int vector_count) {
    VectorIndex* index = (VectorIndex*)malloc(sizeof(VectorIndex));
    index->vectors = vectors;
    index->len = vector_count;
    index->hnsw_graph = NULL;
    index->use_hnsw_optimization = 0;
    return index;
}

VectorIndex* create_hnsw_index(Vector* vectors, int vector_count, int max_connections, 
                              int max_connections_layer_zero, float level_factor) {
    VectorIndex* index = create_index(vectors, vector_count);
    
    // Build HNSW graph with reasonable defaults
    int construction_search_width = max_connections * 2;
    index->hnsw_graph = build_hnsw_graph(vectors, vector_count, max_connections,
                                        max_connections_layer_zero, level_factor, 
                                        construction_search_width);
    index->use_hnsw_optimization = 1;
    
    return index;
}

void free_hnsw_graph(HNSWGraph* graph) {
    if (!graph) return;
    
    for (int node_index = 0; node_index < graph->node_count; node_index++) {
        HNSWNode* node = &graph->nodes[node_index];
        for (int layer = 0; layer <= node->maximum_layer; layer++) {
            free(node->layer_connections[layer]);
        }
        free(node->layer_connections);
        free(node->connection_counts);
        free(node->allocated_connection_sizes);
    }
    
    free(graph->nodes);
    free(graph);
}

void free_index(VectorIndex* index) {
    if (index->hnsw_graph) {
        free_hnsw_graph(index->hnsw_graph);
    }
    free(index);
}