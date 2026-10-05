"""Three-way benchmark: legacy algorithm, pre-hardening commit, and current pipeline.
Separate process per document/strategy; warm-up followed by repeated runs in that process.
"""
import argparse
import json
import resource
import statistics
import subprocess
import sys
import tempfile
import time
from pathlib import Path
sys.path[:0] = [str(Path(__file__).resolve().parents[1]), str(Path(__file__).resolve().parents[1]/'tests')]
from corpus import corpus, legacy


def page_metrics(pages, expected):
    metrics = []
    for i, page in enumerate(pages):
        text = '\n'.join(b['text'] for b in page['blocks'] if not b.get('exclude_from_text', b.get('type') in ('header', 'footer')))
        truth = expected[i] if expected else None
        positions = [text.find(t) for t in truth] if truth else None
        metrics.append({'page': i+1, 'characters': len(text),
                        'coverage': sum(p >= 0 for p in positions)/len(positions) if positions else None,
                        'ordered': all(p >= 0 for p in positions) and positions == sorted(positions) if positions else None,
                        'expected_occurrences': [text.count(t) for t in truth] if truth else None,
                        'strategy': page.get('strategy', 'legacy'), 'ocr': page.get('ocr'),
                        'fallback_requested': page.get('fallback_requested', False),
                        'warnings': page.get('warnings', []), 'quality': page.get('quality'),
                        'duration_ms': page.get('duration_ms')})
    return metrics


def measure(fn, data, expected, runs, warmup):
    for _ in range(warmup): fn(data)
    samples = []
    import pymupdf
    original_ocr = pymupdf.Page.get_textpage_ocr
    ocr_pages = []
    def tracked_ocr(page, *args, **kwargs):
        ocr_pages.append(page.number+1)
        return original_ocr(page, *args, **kwargs)
    pymupdf.Page.get_textpage_ocr = tracked_ocr
    try:
        for _ in range(runs):
            ocr_pages.clear()
            started, cpu = time.perf_counter(), time.process_time()
            result = fn(data)
            elapsed, cpu_elapsed = time.perf_counter()-started, time.process_time()-cpu
            samples.append({'seconds': elapsed, 'cpu_seconds': cpu_elapsed, 'ocr_pages': list(ocr_pages),
                            'peak_rss_kib': resource.getrusage(resource.RUSAGE_SELF).ru_maxrss})
    finally:
        pymupdf.Page.get_textpage_ocr = original_ocr
    pages = result['pages'] if isinstance(result, dict) else result
    times = [sample['seconds'] for sample in samples]
    cpu_times = [sample['cpu_seconds'] for sample in samples]
    median = statistics.median(times)
    return {'runs': runs, 'warmup': warmup, 'seconds_median': median, 'seconds_min': min(times), 'seconds_max': max(times),
            'cpu_seconds_median': statistics.median(cpu_times), 'cpu_seconds_min': min(cpu_times), 'cpu_seconds_max': max(cpu_times),
            'peak_rss_kib': max(sample['peak_rss_kib'] for sample in samples), 'pages_per_second': len(pages)/median,
            'ocr_pages': list(ocr_pages), 'samples': samples, 'pages': page_metrics(pages, expected)}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--legacy-only', action='store_true')
    parser.add_argument('--pdf', type=Path)
    parser.add_argument('--previous-dir', type=Path)
    parser.add_argument('--runs', type=int, default=5)
    parser.add_argument('--warmup', type=int, default=1)
    parser.add_argument('--measure', choices=['before', 'previous', 'after'])
    parser.add_argument('--input', type=Path)
    args = parser.parse_args()
    if args.runs < 1 or args.warmup < 0: parser.error('runs must be positive and warmup nonnegative')
    if args.measure:
        request = json.loads(args.input.read_text())
        data = Path(request['pdf']).read_bytes()
        if args.measure == 'previous': sys.path.insert(0, request['previous_dir'])
        if args.measure != 'before':
            from pipeline import extract_document
            fn = extract_document
        else:
            fn = legacy
        print(json.dumps(measure(fn, data, request['expected'], args.runs, args.warmup)))
        sys.exit()
    cases = {args.pdf.name: (args.pdf.read_bytes(), None)} if args.pdf else corpus()
    rows = {}
    for name, (data, expected) in cases.items():
        rows[name] = {}
        with tempfile.TemporaryDirectory() as directory:
            pdf = Path(directory)/'input.pdf'
            pdf.write_bytes(data)
            request = Path(directory)/'request.json'
            request.write_text(json.dumps({'pdf': str(pdf), 'expected': expected,
                                          'previous_dir': str(args.previous_dir.resolve()) if args.previous_dir else None}))
            strategies = ('before',) if args.legacy_only else ('before', 'previous', 'after') if args.previous_dir else ('before', 'after')
            for strategy in strategies:
                result = subprocess.run([sys.executable, __file__, '--measure', strategy, '--input', str(request),
                                         '--runs', str(args.runs), '--warmup', str(args.warmup)],
                                        capture_output=True, text=True, timeout=300, check=True)
                rows[name][strategy] = json.loads(result.stdout)
    print(json.dumps({'measurement': 'parser stages only; warm process; no semantic ground truth for --pdf; RSS includes warm-up',
                      'documents': rows}, indent=2))
