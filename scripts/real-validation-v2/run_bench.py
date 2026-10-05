"""Private corpus audit through the normal worker limits; output is Git-ignored."""
import argparse
import hashlib
import io
import shutil
import subprocess
import tarfile
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument('--image', default='pdf-reader-extractor-hardening:latest')
parser.add_argument('--baseline', default='ecb2d0fe1f1a58b04e81601be05f68968acdc84b')
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
out = root / 'artifacts/real-validation-v2'
out.mkdir(parents=True, exist_ok=True)
for name in ['profile_parent.py', 'profile_worker.py']:
    shutil.copyfile(Path(__file__).parent / name, out / name)
previous = out / 'before-extractor'
previous.mkdir(exist_ok=True)
archive = subprocess.run(['git', 'archive', args.baseline, 'extractor'], cwd=root,
                         check=True, stdout=subprocess.PIPE).stdout
with tarfile.open(fileobj=io.BytesIO(archive)) as source:
    for member in source.getmembers():
        member.name = str(Path(member.name).relative_to('extractor'))
        source.extract(member, previous, filter='data')
unique = {}
for path in sorted((root / 'validation-corpus').rglob('*')):
    if path.is_file() and path.suffix.lower() == '.pdf':
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        if digest in unique:
            print('duplicate:', path.name, 'of', unique[digest].name, flush=True)
        else:
            unique[digest] = path
for phase in ['before', 'after']:
    app = root / 'extractor' if phase == 'after' else previous
    for path in unique.values():
        relative = path.relative_to(root / 'validation-corpus')
        command = ['docker', 'run', '--rm', '--memory=2g', '--cpus=2',
                   '-v', f'{app}:/app:ro', '-v', f'{out}:/out',
                   '-v', f'{root}/validation-corpus:/corpus:ro', args.image,
                   'python', '/out/profile_parent.py', f'/corpus/{relative}', phase]
        result = subprocess.run(command, check=True, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, text=True, timeout=110)
        print(result.stdout.strip(), flush=True)
