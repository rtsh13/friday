#!/usr/bin/env python3
import os
import json
from pathlib import Path


class DocumentProcessor:
    def __init__(self, output_dir):
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(parents=True, exist_ok=True)
        self.chunk_size = 512
        self.chunk_overlap = 50
        self.chunk_counter = 0  # Add global counter

    def process_file(self, file_path):
        try:
            with open(file_path, "r", encoding="utf-8", errors="ignore") as f:
                content = f.read()
        except:
            return []

        chunks = self._chunk_text(content)
        return self._create_metadata(chunks, file_path)

    def _chunk_text(self, text):
        words = text.split()
        chunks = []
        for i in range(0, len(words), self.chunk_size - self.chunk_overlap):
            chunk = " ".join(words[i : i + self.chunk_size])
            if len(chunk) > 50:
                chunks.append(chunk)
        return chunks

    def _create_metadata(self, chunks, source):
        processed = []
        category = self._categorize(source)
        for i, chunk in enumerate(chunks):
            self.chunk_counter += 1  # Increment global counter
            processed.append(
                {
                    "id": self.chunk_counter,  # Use integer ID
                    "content": chunk,
                    "metadata": {
                        "source": str(source),
                        "chunk_index": i,
                        "category": category,
                    },
                }
            )
        return processed

    def _categorize(self, path):
        p = str(path).lower()
        if "grpc" in p:
            return "grpc"
        if "gnmi" in p:
            return "gnmi"
        if "yang" in p or "openconfig" in p:
            return "yang"
        if "debug" in p:
            return "debugging"
        if "network" in p or "tcp" in p:
            return "network"
        return "general"

    def process_directory(self, input_dir):
        all_chunks = []
        for file_path in Path(input_dir).rglob("*.md"):
            chunks = self.process_file(str(file_path))
            all_chunks.extend(chunks)
            if chunks:
                print(f" {file_path.name}: {len(chunks)} chunks")
        return all_chunks

    def quality_filter(self, chunks):
        filtered = []
        for chunk in chunks:
            content = chunk["content"]
            if len(content) < 50:
                continue
            if content.count(" ") / len(content) < 0.05:
                continue
            alpha = sum(c.isalpha() for c in content)
            if alpha / len(content) < 0.3:
                continue
            filtered.append(chunk)
        return filtered

    def save(self, chunks, filename):
        path = self.output_dir / filename
        with open(path, "w", encoding="utf-8") as f:
            json.dump(chunks, f, indent=2)
        print(f"Saved {len(chunks)} chunks to {path}")


def main():
    processor = DocumentProcessor("../data/processed")
    categories = [
        ("../data/raw-docs/grpc/doc", "grpc_chunks.json"),
        ("../data/raw-docs/gnmi", "gnmi_chunks.json"),
        ("../data/raw-docs/public", "yang_chunks.json"),
    ]

    all_chunks = []
    for input_dir, output_file in categories:
        if os.path.exists(input_dir):
            print(f"\nProcessing: {input_dir}")
            chunks = processor.process_directory(input_dir)
            filtered = processor.quality_filter(chunks)
            print(f"Filtered: {len(chunks)} → {len(filtered)}")
            processor.save(filtered, output_file)
            all_chunks.extend(filtered)

    processor.save(all_chunks, "all_chunks.json")
    print(f"\nTotal: {len(all_chunks)} chunks")


if __name__ == "__main__":
    main()
