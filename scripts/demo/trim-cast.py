#!/usr/bin/env python3
"""
trim-cast.py — Shorten an asciinema cast file (v2 or v3).

Strategy:
  1. Normalise timestamps to relative deltas (v2 uses absolute timestamps;
     v3 already uses deltas — both are handled automatically).
  2. Cap any idle gap > MAX_GAP seconds (default 1.0s).
  3. Scale all timestamps by 1/SPEED (default 65x).
  4. Write output in v3 (relative timestamp) format.

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

    version = header.get('version', 2)

    # v2 stores absolute timestamps; convert to relative deltas so the cap
    # and speed multiplier operate on inter-event gaps, not wall-clock times.
    if version == 2:
        deltas = []
        prev = 0.0
        for e in events:
            deltas.append(e[0] - prev)
            prev = e[0]
    else:
        # v3 already stores relative deltas.
        deltas = [e[0] for e in events]

    total_in = sum(deltas)
    total_out = sum(min(d, max_gap) / speed for d in deltas)
    print(f"Input:  {total_in:.1f}s = {total_in/60:.1f} min  ({len(events)} events, v{version})")
    print(f"Output: {total_out:.1f}s = {total_out/60:.2f} min  (cap={max_gap}s, speed={speed}x, v3)")

    # Output header: always v3 (relative timestamps), drop idle_time_limit
    # since gaps are already baked in.
    out_header = dict(header)
    out_header['version'] = 3
    out_header.pop('idle_time_limit', None)

    with open(output_path, 'w') as f:
        f.write(json.dumps(out_header) + '\n')
        for delta, e in zip(deltas, events):
            new_ts = round(min(delta, max_gap) / speed, 6)
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
