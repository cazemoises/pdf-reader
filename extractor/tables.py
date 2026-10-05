"""Cheap ruled-grid candidate detection before the substantially more expensive table parser."""


def has_ruled_grid(paths):
    horizontal = set()
    vertical = set()
    for path in paths:
        if path.get('type') not in ('s', 'fs'):
            continue  # Filled backgrounds/images are not table rulings.
        for item in path['items']:
            if item[0] == 'l':
                a, b = item[1:3]
                if a.y == b.y and a.x != b.x:
                    horizontal.add(a.y)
                if a.x == b.x and a.y != b.y:
                    vertical.add(a.x)
            elif item[0] == 're':
                r = item[1]
                horizontal.update((r.y0, r.y1))
                vertical.update((r.x0, r.x1))
    # A multi-cell ruled table requires at least two edges on each axis,
    # plus an internal separator. A single page border is insufficient.
    return len(horizontal) >= 2 and len(vertical) >= 2 and max(len(horizontal), len(vertical)) >= 3
