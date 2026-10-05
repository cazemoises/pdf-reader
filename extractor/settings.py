"""Operational budgets and conservative detection policy; see docs/extraction.md."""
from dataclasses import dataclass
import os


@dataclass(frozen=True)
class Settings:
    max_bytes: int = 32 * 1024 * 1024
    max_pages: int = 500
    max_output_bytes: int = 32 * 1024 * 1024
    timeout_seconds: int = 90
    memory_bytes: int = 1536 * 1024 * 1024
    max_page_pixels: int = 20_000_000
    max_blocks: int = 20_000
    ocr_dpi: int = 150
    ocr_language: str = 'por+eng'
    ocr_enabled: bool = True
    image_area_trigger: float = 0.5
    invalid_ratio_trigger: float = 0.05
    margin_fraction: float = 0.06

    @classmethod
    def from_env(cls):
        return cls(ocr_enabled=os.getenv('PDF_OCR_ENABLED', 'true').lower() == 'true',
                   ocr_language=os.getenv('PDF_OCR_LANGUAGE', 'por+eng'))
