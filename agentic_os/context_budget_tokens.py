"""Shared deterministic token proxy for context-budget measurements."""

from __future__ import annotations

CHARS_PER_TOKEN = 4
TOKENIZER_NOTE = "tokens = chars/4 proxy (v1; swap for the qwen tokenizer later)"


def count_tokens(text: str) -> int:
    """Estimate tokens for text with the repository's hermetic chars/4 proxy."""
    return -(-len(text) // CHARS_PER_TOKEN)
