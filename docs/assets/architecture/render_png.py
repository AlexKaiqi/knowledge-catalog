#!/usr/bin/env python3
"""Render the editable architecture SVGs with the existing macOS tools."""

import argparse
from pathlib import Path
import shutil
import struct
import subprocess
import tempfile
import xml.etree.ElementTree as ET


def render(source: Path, scale: int) -> None:
    tree = ET.parse(source)
    svg = tree.getroot()
    width, height = (int(float(v)) for v in svg.attrib['viewBox'].split()[2:])
    side = max(width, height)
    # Quick Look uses a square thumbnail canvas. Center the temporary render
    # source so sips can remove that padding with its ordinary centered crop.
    svg.set('width', str(side))
    svg.set('height', str(side))
    svg.set('viewBox', f'0 0 {side} {side}')
    drawing = ET.Element('{http://www.w3.org/2000/svg}g', {'transform': f'translate({(side - width) / 2} {(side - height) / 2})'})
    for child in list(svg):
        if child.tag.rsplit('}', 1)[-1] in {'g', 'rect', 'path', 'text'}:
            svg.remove(child)
            drawing.append(child)
    svg.append(drawing)
    ET.register_namespace('', 'http://www.w3.org/2000/svg')
    target = source.with_suffix('.png')
    with tempfile.TemporaryDirectory(prefix='kc-architecture-') as temporary:
        stage = Path(temporary)
        padded = stage / source.name
        tree.write(padded, encoding='utf-8', xml_declaration=True)
        subprocess.run(['qlmanage', '-t', '-s', str(side * scale), '-o', str(stage), str(padded)], check=True, stdout=subprocess.DEVNULL)
        thumbnail = padded.with_suffix('.svg.png')
        subprocess.run(['sips', '--cropToHeightWidth', str(height * scale), str(width * scale), str(thumbnail), '--out', str(target)], check=True, stdout=subprocess.DEVNULL)
    pixels = struct.unpack('>II', target.read_bytes()[16:24])
    if pixels != (width * scale, height * scale):
        raise RuntimeError(f'{target}: unexpected rendered bounds {pixels}')
    print(f'{target.name}: {pixels[0]} × {pixels[1]}')


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('svg', nargs='*', type=Path)
    parser.add_argument('--scale', type=int, default=2)
    options = parser.parse_args()
    if options.scale < 1:
        parser.error('--scale must be positive')
    for command in ('qlmanage', 'sips'):
        if shutil.which(command) is None:
            parser.error(f'{command} is required; use an SVG renderer already available in your environment')
    sources = options.svg or sorted(Path(__file__).parent.glob('*.svg'))
    for source in sources:
        render(source.resolve(), options.scale)


if __name__ == '__main__':
    main()
