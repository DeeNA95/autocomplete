#!/bin/bash
set -e

# Get the absolute path to the directory containing this script
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# Get the absolute path to the project root (one level up from the script's directory)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)

echo "Removing old server executables..."
rm -f "$PROJECT_ROOT/extension/autocomplete-extension/server"
rm -f "$PROJECT_ROOT/extension/autocomplete-extension/backend/server"

echo "Cleaning any development test/demo executables..."
cd "$PROJECT_ROOT/backend/internal/storage"
rm -f test_hnsw test_vector_search vector_search_demo vector_search_test hnsw_test

echo "Building C library..."
cd "$PROJECT_ROOT/backend/internal/storage"

# Safety check: Ensure we don't accidentally compile test files
if [ -f "test_hnsw" ] || [ -f "vector_search_demo" ]; then
    echo "⚠️  Warning: Found test/demo executables, removing them..."
    rm -f test_hnsw test_vector_search vector_search_demo vector_search_test hnsw_test
fi

# Verify we have the required source files
if [ ! -f "vector_search.c" ]; then
    echo "❌ Error: vector_search.c not found"
    exit 1
fi

if [ ! -f "vector_search.h" ]; then
    echo "❌ Error: vector_search.h not found"
    exit 1
fi

# Build only the production library (NOT test/demo files)
echo "Compiling vector_search.c (production build only)..."
gcc -c -o vector_search.o vector_search.c -Wall -Wextra -std=c99 -O2

echo "Creating static library..."
ar rcs libvectorsearch.a vector_search.o

# Verify library was created
if [ ! -f "libvectorsearch.a" ]; then
    echo "❌ Error: Failed to create libvectorsearch.a"
    exit 1
fi

echo "✅ C library built successfully (production build)"

echo "Building Go backend..."
export GIN_MODE=release # Set GIN_MODE to release for production builds
# Change into the Go module directory
cd "$PROJECT_ROOT/backend"
# Build the main package
GIN_MODE=release go build  -o "$PROJECT_ROOT/extension/autocomplete-extension/server" ./cmd/server

echo "Build complete."

# Final safety check: Ensure no test/demo executables exist
cd "$PROJECT_ROOT/backend/internal/storage"
if [ -f "test_hnsw" ] || [ -f "vector_search_demo" ] || [ -f "test_vector_search" ]; then
    echo "❌ Error: Test/demo executables found after build - this should not happen"
    echo "   Found files:"
    ls -la test_hnsw vector_search_demo test_vector_search 2>/dev/null || true
    echo "   These files are for development only and should not be in production builds"
    echo ""
    echo "To build test/demo executables for development:"
    echo "  cd backend/internal/storage"
    echo "  make -f Makefile.dev test    # Build and run tests"
    echo "  make -f Makefile.dev demo    # Build and run demo"
    exit 1
fi

echo "✅ Production build verified - no test/demo executables present"

# BAAI/bge-small-en-v1.5