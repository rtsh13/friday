#!/usr/bin/env python3
from qdrant_client import QdrantClient
from sentence_transformers import SentenceTransformer


def main():
    client = QdrantClient(host="localhost", port=6333)
    model = SentenceTransformer("sentence-transformers/all-MiniLM-L6-v2")

    queries = ["How do I debug gRPC packet drops?", "TCP buffer tuning"]

    for query in queries:
        print(f"\n{'=' * 70}\nQuery: {query}\n{'=' * 70}\n")
        vector = model.encode(query).tolist()
        results = client.search(
            collection_name="telemetry_docs",
            query_vector=vector,
            limit=3,
            score_threshold=0.7,
        )
        for i, r in enumerate(results, 1):
            print(f"[{i}] Score: {r.score:.3f}")
            print(f"    {r.payload['content'][:150]}...\n")


if __name__ == "__main__":
    main()
