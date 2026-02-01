#!/usr/bin/env python3
import json
from qdrant_client import QdrantClient
from qdrant_client.models import Distance, VectorParams, PointStruct


def main():
    with open("../data/processed/chunks_with_embeddings.json") as f:
        chunks = json.load(f)
    print(f"Loaded {len(chunks)} chunks")

    # Verify embeddings exist
    if "embedding" not in chunks[0]:
        print("ERROR: No embeddings found in chunks!")
        return

    print(f"Embedding dimension: {len(chunks[0]['embedding'])}")

    client = QdrantClient(host="localhost", port=6333)
    collection = "telemetry_docs"

    # Delete and recreate collection
    try:
        client.delete_collection(collection)
        print("Deleted existing collection")
    except:
        pass

    client.create_collection(
        collection_name=collection,
        vectors_config=VectorParams(size=384, distance=Distance.COSINE),
    )
    print("Created collection")

    # Create points with explicit vector field
    points = []
    for chunk in chunks:
        point = PointStruct(
            id=chunk["id"],
            vector=chunk["embedding"],  # Must be a list of floats
            payload={
                "content": chunk["content"],
                "source": chunk["metadata"]["source"],
                "category": chunk["metadata"]["category"],
            },
        )
        points.append(point)

    # Upload in batches
    batch_size = 100
    for i in range(0, len(points), batch_size):
        batch = points[i : i + batch_size]
        client.upsert(collection_name=collection, points=batch)
        print(f"Indexed {min(i + batch_size, len(points))}/{len(points)}", end="\r")

    print(f"\n✓ Indexed {len(points)} chunks")

    # Verify
    info = client.get_collection(collection)
    print(f"Collection points: {info.points_count}")


if __name__ == "__main__":
    main()
