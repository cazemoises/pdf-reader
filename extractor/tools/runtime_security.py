"""Verify the actual production container restrictions, not just Dockerfile declarations."""
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

assert os.geteuid() == 10001, 'parser service still runs as root'
status = dict(line.split(':', 1) for line in Path('/proc/self/status').read_text().splitlines() if ':' in line)
assert int(status['CapEff'].strip(), 16) == 0
assert status['NoNewPrivs'].strip() == '1'
try:
    Path('/app/forbidden-write').write_text('must not succeed')
except OSError:
    pass
else:
    raise AssertionError('application filesystem is writable')
with tempfile.NamedTemporaryFile() as file:
    file.write(b'bounded temporary data')
    temporary = file.name
assert not Path(temporary).exists()
child_uid = subprocess.check_output([sys.executable, '-c', 'import os; print(os.geteuid())'], text=True).strip()
assert child_uid == '10001'
assert os.environ['OMP_THREAD_LIMIT'] == '1'
limits = {name: Path('/sys/fs/cgroup', name).read_text().strip() for name in ('memory.max', 'cpu.max', 'pids.max') if Path('/sys/fs/cgroup', name).exists()}
print(json.dumps({'uid': os.geteuid(), 'child_uid': child_uid, 'capabilities': status['CapEff'].strip(),
                  'no_new_privileges': True, 'read_only_verified': True, 'tmp_cleanup_verified': True, 'cgroup_limits': limits}))
