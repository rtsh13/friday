#!/usr/bin/env python3
from sentence_transformers import SentenceTransformer
import torch
import json
import os


def export_model():
    print("Loading model...")
    model = SentenceTransformer("sentence-transformers/all-MiniLM-L6-v2")

    print("Exporting to ONNX...")
    dummy_input = {
        "input_ids": torch.randint(0, 30522, (1, 128)),
        "attention_mask": torch.ones(1, 128, dtype=torch.long),
    }

    torch.onnx.export(
        model[0].auto_model,
        (dummy_input["input_ids"], dummy_input["attention_mask"]),
        "minilm-l6-v2.onnx",
        input_names=["input_ids", "attention_mask"],
        output_names=["output"],
        dynamic_axes={
            "input_ids": {0: "batch", 1: "sequence"},
            "attention_mask": {0: "batch", 1: "sequence"},
            "output": {0: "batch", 1: "sequence"},
        },
        opset_version=14,
    )
    print("✓ Model exported")

    tokenizer = model.tokenizer
    vocab = tokenizer.get_vocab()
    with open("vocab.json", "w") as f:
        json.dump(vocab, f)
    print("✓ Vocabulary saved")


if __name__ == "__main__":
    export_model()
