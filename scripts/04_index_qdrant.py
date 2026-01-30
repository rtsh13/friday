#!/usr/bin/env python3
import json
from qdrant_client import QdrantClient
from qdrant_client.models import Distance, VectorParams, PointStruct


def main():
    with open("../data/processed/chunks_with_embeddings.json") as f:
        chunks = json.load(f)
    print(f"Loaded {len(chunks)} chunks")

    client = QdrantClient(host="localhost", port=6333)
    collection = "telemetry_docs"

    try:
        client.delete_collection(collection)
    except:
        pass

    client.create_collection(
        collection_name=collection,
        vectors_config=VectorParams(size=384, distance=Distance.COSINE),
    )
    print(f"Created collection")

    points = [
        PointStruct(
            id=c["id"],
            vector=c["embedding"],
            payload={
                "content": c["content"],
                "source": c["metadata"]["source"],
                "category": c["metadata"]["category"],
            },
        )
        for c in chunks
    ]

    for i in range(0, len(points), 100):
        client.upsert(collection_name=collection, points=points[i : i + 100])
        print(f"Indexed {min(i + 100, len(points))}/{len(points)}", end="\r")

    print(f"\n✓ Indexed {len(points)} chunks")


if __name__ == "__main__":
    main()
