"""Diagnostics are observable signals, not a claim of semantic accuracy."""
import unicodedata


def evaluate(blocks):
    text = '\n'.join(b['text'] for b in blocks)
    invalid = sum(c == '\ufffd' or (unicodedata.category(c) == 'Cc' and not c.isspace()) for c in text)
    non_whitespace = sum(not c.isspace() for c in text)
    ratio = invalid / max(1, non_whitespace)
    values = [b['text'] for b in blocks if b['text']]
    duplicates = len(values)-len(set(values))
    return {'characters': len(text), 'invalid_characters': invalid, 'invalid_ratio': ratio, 'non_whitespace_characters': non_whitespace, 'empty': non_whitespace == 0,
            'duplicate_blocks': duplicates, 'score': None if not non_whitespace else round(1-ratio, 4),
            'score_kind': 'character_integrity', 'ocr_confidence': None}
