#!/usr/bin/env python3
"""
trim-cast.py — Shorten an asciinema v3 cast file.

Strategy:
  1. Cap any idle gap > MAX_GAP seconds (default 1.0s)
  2. Scale all timestamps by 1/SPEED (default 65x)

Usage:
  python3 demo/trim-cast.py [input.cast] [output.cast] [--cap SECONDS] [--speed FACTOR]

Defaults:
  input   demo/forge-demo.cast
  output  demo/forge-demo-short.cast
  --cap   1.0
  --speed 65
"""

import json
import sys
import argparse

def trim(input_path, output_path, max_gap, speed):
    header = None
    events = []

    with open(input_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            if line.startswith('{'):
                header = json.loads(line)
            elif line.startswith('['):
                events.append(json.loads(line))

    if header is None:
        print("ERROR: no header found", file=sys.stderr)
        sys.exit(1)

    # Update header: remove idle_time_limit (already baked in), note new duration
    out_header = dict(header)
    out_header.pop('idle_time_limit', None)

    total_in = sum(e[0] for e in events)
    total_out = sum(min(e[0], max_gap) / speed for e in events)
    print(f"Input:  {total_in:.1f}s = {total_in/60:.1f} min  ({len(events)} events)")
    print(f"Output: {total_out:.1f}s = {total_out/60:.2f} min  (cap={max_gap}s, speed={speed}x)")

    with open(output_path, 'w') as f:
        f.write(json.dumps(out_header) + '\n')
        for e in events:
            new_ts = round(min(e[0], max_gap) / speed, 6)
            f.write(json.dumps([new_ts] + e[1:]) + '\n')

    print(f"Written: {output_path}")

def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('input',  nargs='?', default='demo/forge-demo.cast')
    parser.add_argument('output', nargs='?', default='demo/forge-demo-short.cast')
    parser.add_argument('--cap',   type=float, default=1.0,  help='Max gap to keep in seconds (default: 1.0)')
    parser.add_argument('--speed', type=float, default=65.0, help='Speed multiplier (default: 65)')
    args = parser.parse_args()

    trim(args.input, args.output, args.cap, args.speed)

if __name__ == '__main__':
    main()
