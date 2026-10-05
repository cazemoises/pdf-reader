"""Reproducible per-document comparisons in separate processes, including native peak RSS.
Run from extractor: python tools/benchmark.py [--legacy-only] [--pdf PATH].
"""
import argparse
import json
import resource
import subprocess
import sys
import tempfile
import time
from pathlib import Path
sys.path[:0] = [str(Path(__file__).resolve().parents[1]), str(Path(__file__).resolve().parents[1] / 'tests')]
from corpus import corpus, legacy


def measure(fn, data, expected):
    started = time.perf_counter()
    cpu = time.process_time()
    result = fn(data)
    pages = result['pages'] if isinstance(result, dict) else result
    metrics = []
    for i, page in enumerate(pages):
        text = '\n'.join(b['text'] for b in page['blocks'] if b.get('type') not in ('header', 'footer'))
        truth = expected[i] if expected is not None else None
        positions = [text.find(t) for t in truth] if truth else None
        metrics.append({'page': i+1, 'characters': len(text),
                        'coverage': sum(p >= 0 for p in positions)/len(positions) if positions else None,
                        'ordered': all(p >= 0 for p in positions) and positions == sorted(positions) if positions else None,
                        'strategy': page.get('strategy', 'legacy'), 'fallback_requested': page.get('fallback_requested', False),
                        'warnings': page.get('warnings', []), 'quality': page.get('quality'),
                        'duration_ms': page.get('duration_ms')})
    return {'seconds': time.perf_counter()-started, 'cpu_seconds': time.process_time()-cpu,
            'peak_rss_kib': resource.getrusage(resource.RUSAGE_SELF).ru_maxrss, 'pages': metrics}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--legacy-only', action='store_true')
    parser.add_argument('--pdf', type=Path)
    parser.add_argument('--measure', choices=['before', 'after'])
    parser.add_argument('--input', type=Path)
    args = parser.parse_args()
    if args.measure:
        request = json.loads(args.input.read_text())
        data = Path(request['pdf']).read_bytes()
        if args.measure == 'after':
            from pipeline import extract_document
            fn = extract_document
        else:
            fn = legacy
        print(json.dumps(measure(fn, data, request['expected'])))
        sys.exit()
    cases = {args.pdf.name: (args.pdf.read_bytes(), None)} if args.pdf else corpus()
    rows = {}
    for name, (data, expected) in cases.items():
        rows[name] = {}
        with tempfile.TemporaryDirectory() as directory:
            pdf = Path(directory)/'input.pdf'
            pdf.write_bytes(data)
            request = Path(directory)/'request.json'
            request.write_text(json.dumps({'pdf': str(pdf), 'expected': expected}))
            for strategy in ('before',) if args.legacy_only else ('before', 'after'):
                result = subprocess.run([sys.executable, __file__, '--measure', strategy, '--input', str(request)],
                                        capture_output=True, text=True, timeout=120, check=True)
                rows[name][strategy] = json.loads(result.stdout)
    print(json.dumps({'measurement': 'single run; parser stages only; separate process RSS; no semantic ground truth for --pdf',
                      'documents': rows}, indent=2))
