#!/usr/bin/env python3
from qdrant_client import QdrantClient
from sentence_transformers import SentenceTransformer


def main():
    client = QdrantClient(host="localhost", port=6333)
    model = SentenceTransformer("sentence-transformers/all-MiniLM-L6-v2")

    # First, check collection info
    collection_info = client.get_collection("telemetry_docs")
    print(f"Collection has {collection_info.points_count} points\n")

    queries = ["gRPC", "network"]

    for query in queries:
        print(f"\n{'=' * 70}\nQuery: {query}\n{'=' * 70}\n")
        vector = model.encode(query).tolist()

        results = client.query_points(
            collection_name="telemetry_docs",
            query=vector,
            limit=5,
            score_threshold=0.3,  # Lower threshold
        ).points

        if not results:
            print("No results found.")
            continue

        for i, r in enumerate(results, 1):
            print(f"[{i}] Score: {r.score:.3f}")
            print(f"    Category: {r.payload.get('category', 'N/A')}")
            print(f"    Source: {r.payload.get('source', 'N/A')}")
            print(f"    {r.payload['content'][:100]}...\n")


if __name__ == "__main__":
    main()
