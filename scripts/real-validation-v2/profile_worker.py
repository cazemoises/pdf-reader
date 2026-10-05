import sys,time,json,atexit,runpy,pymupdf
sys.path.insert(0,'/app')
counts={'explicit_renders':0,'ocr_calls':0,'ocr_full':0,'ocr_partial':0,'ocr_seconds':0}
original_pix=pymupdf.Page.get_pixmap;original_ocr=pymupdf.Page.get_textpage_ocr

def pix(self,*args,**kwargs):
 counts['explicit_renders']+=1
 return original_pix(self,*args,**kwargs)

def ocr(self,*args,**kwargs):
 counts['ocr_calls']+=1;counts['ocr_full' if kwargs.get('full') else 'ocr_partial']+=1;start=time.perf_counter()
 try:return original_ocr(self,*args,**kwargs)
 finally:counts['ocr_seconds']+=time.perf_counter()-start
pymupdf.Page.get_pixmap=pix;pymupdf.Page.get_textpage_ocr=ocr
atexit.register(lambda:sys.stderr.write('\nAUDIT_METRICS '+json.dumps(counts)+'\n'))
runpy.run_path('/app/worker.py',run_name='__main__')
