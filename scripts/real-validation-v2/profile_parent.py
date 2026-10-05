import sys,json,subprocess,resource,time
from pathlib import Path
sys.path.insert(0,'/app')
import main
from fastapi import HTTPException
original=subprocess.run;measurements={}

def measured(args,*pos,**kwargs):
 if args[-1].endswith('worker.py'):
  args=[args[0],'/out/profile_worker.py'];r=original(args,*pos,**kwargs)
  errors=kwargs['stderr'];errors.seek(0)
  for line in errors.read().decode(errors='replace').splitlines():
   if line.startswith('AUDIT_METRICS '):measurements.update(json.loads(line.removeprefix('AUDIT_METRICS ')))
  return r
 return original(args,*pos,**kwargs)
main.subprocess.run=measured
path=Path(sys.argv[1]);phase=sys.argv[2];start=time.perf_counter()
try:
 result=main.run_worker(path.read_bytes());status='production_worker_ok'
except HTTPException as e:
 result=None;status=f'production_worker_http_{e.status_code}'
usage=resource.getrusage(resource.RUSAGE_CHILDREN)
metrics={'wall_seconds':time.perf_counter()-start,'cpu_seconds':usage.ru_utime+usage.ru_stime,'peak_rss_kib':usage.ru_maxrss,'status':status,**measurements}
import hashlib
key=hashlib.sha256(path.read_bytes()).hexdigest()[:16]
Path('/out',f'{phase}-{key}.json').write_text(json.dumps({'file':path.name,'metrics':metrics,'result':result},ensure_ascii=False))
print(phase,key,json.dumps(metrics),flush=True)
