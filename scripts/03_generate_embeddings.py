#!/usr/bin/env python3
import json
from sentence_transformers import SentenceTransformer
from tqdm import tqdm


def main():
    print("Loading model...")
    model = SentenceTransformer("sentence-transformers/all-MiniLM-L6-v2")

    with open("../data/processed/all_chunks.json") as f:
        chunks = json.load(f)
    print(f"Loaded {len(chunks)} chunks")

    texts = [c["content"] for c in chunks]
    embeddings = []

    for i in tqdm(range(0, len(texts), 32), desc="Generating"):
        batch = texts[i : i + 32]
        batch_emb = model.encode(batch, show_progress_bar=False)
        embeddings.extend(batch_emb)

    for chunk, emb in zip(chunks, embeddings):
        chunk["embedding"] = emb.tolist()

    output = "../data/processed/chunks_with_embeddings.json"
    with open(output, "w") as f:
        json.dump(chunks, f, indent=2)
    print(f"✓ Saved to {output}")


if __name__ == "__main__":
    main()
