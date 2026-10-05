"""Original CC0 adversarial PDFs. Native content is exact ground truth; OCR has declared tolerances."""
import pymupdf


def pdf_page(draw, width=600, height=800, rotation=0):
    with pymupdf.open() as doc:
        page = doc.new_page(width=width, height=height)
        draw(page)
        page.set_rotation(rotation)
        return doc.tobytes(deflate=True)


def raster(text=None, decorative=False, low_resolution=False, color=(0, 0, 0)):
    with pymupdf.open() as doc:
        page = doc.new_page(width=600, height=600)
        if decorative:
            page.draw_rect(page.rect, fill=(0.2, 0.4, 0.8), color=None)
            page.draw_circle((300, 300), 180, fill=(0.8, 0.6, 0.2), color=None)
        elif text:
            page.insert_text((60, 210), text, fontsize=26, color=color)
        return page.get_pixmap(matrix=pymupdf.Matrix(0.5 if low_resolution else 1.5, 0.5 if low_resolution else 1.5)).tobytes('png')


def image_pdf(text='AÇÃO, CAFÉ, CORAÇÃO', native=None, decorative=False, small=False, rotation=0, low_resolution=False, searchable=False):
    image = raster(text, decorative, low_resolution)
    def draw(page):
        area = pymupdf.Rect(50, 200, 350, 500) if small else pymupdf.Rect(0, 100, 600, 700)
        page.insert_image(area, stream=image)
        if native:
            page.insert_text((50, 60), native)
        if searchable:
            page.insert_text((60, 310), text, fontsize=26, render_mode=3)
    return pdf_page(draw, rotation=rotation)


def columns(count=3, title=False):
    expected = ['A1', 'A2', 'B1', 'B2', 'C1', 'C2'][:2*count]
    def draw(page):
        if title:
            page.insert_text((40, 50), 'A heading spanning the document columns', fontsize=24)
        for i in reversed(range(count*2)):
            page.insert_text((40+(i//2)*180, 100+(i%2)*40), expected[i])
    return pdf_page(draw), (['A heading spanning the document columns'] if title else [])+expected


def ruled_table(multiline=False, near_prose=False, empty=False):
    def draw(page):
        for x in (50, 250, 450):
            page.draw_line((x, 100), (x, 200))
        for y in (100, 150, 200):
            page.draw_line((50, y), (450, y))
        page.insert_text((60, 125), 'Name')
        page.insert_text((260, 125), 'Value')
        page.insert_text((60, 168), 'Alpha\nBeta' if multiline else 'Alpha')
        if not empty:
            page.insert_text((260, 168), '42')
        if near_prose:
            # Same PDF text block can include a table cell followed by external prose.
            page.insert_text((60, 195), 'Cell ending\nOutside prose')
    return pdf_page(draw)


def margins():
    with pymupdf.open() as doc:
        for n in range(5):
            page = doc.new_page(width=600, height=800)
            if n in (0, 4):
                page.insert_text((50, 100), 'Cover or appendix')
            else:
                page.insert_text((50, 25), 'Repeated header')
                page.insert_text((50, 100), 'Repeated legitimate body')
                page.insert_text((50, 700), 'Repeated footer')
            page.insert_text((50, 780), str(n+1))
        return doc.tobytes()


def corrupted_font(nul=False):
    def draw(page):
        font = pymupdf.Font('cjk')
        xref = page.insert_font(fontname='AuditFont', fontbuffer=font.buffer)
        phrase = 'ACAO CORRETA'
        page.insert_text((50, 120), phrase, fontsize=24, fontname='AuditFont')
        pairs = []
        for char in sorted(set(phrase)):
            target = ('0000' if nul else 'FFFD') if char == 'A' else f'{ord(char):04X}'
            pairs.append(f'<{font.has_glyph(ord(char)):04X}> <{target}>')
        mapping = ('/CIDInit /ProcSet findresource begin 12 dict begin begincmap '
                   '/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def '
                   '/CMapName /Test def /CMapType 2 def 1 begincodespacerange <0000> <FFFF> '
                   'endcodespacerange '+str(len(pairs))+' beginbfchar '+' '.join(pairs)+
                   ' endbfchar endcmap CMapName currentdict /CMap defineresource pop end end').encode()
        cmap = int(page.parent.xref_get_key(xref, 'ToUnicode')[1].split()[0])
        page.parent.update_stream(cmap, mapping)
    return pdf_page(draw)
