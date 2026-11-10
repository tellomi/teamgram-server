#!/usr/bin/env python3
"""Read-only, unauthenticated obfuscated2 req_pq_multi -> resPQ probe.
Requires Python 3 and openssl CLI; no account, persistent auth key, or secret.
Only a transport + first MTProto handshake round-trip, NOT a full DH/login test.
Run via stdin over SSH to avoid writing any file on the target.

Does not print the ephemeral AES-CTR key.
"""
import argparse
import datetime
import json
import secrets
import socket
import struct
import subprocess
import sys
import time


def ctr(data, key, iv):
    # Per-connection ephemeral random keys only; never persist or print them.
    p = subprocess.run(['openssl', 'enc', '-aes-256-ctr', '-K', key.hex(),
                        '-iv', iv.hex(), '-nopad'], input=data,
                       capture_output=True, timeout=5)
    if p.returncode:
        raise RuntimeError('openssl AES-CTR failed')
    return p.stdout


def looks_like_preread_wait(h):
    # nginx ssl_preread waits for a full TLS record when the first bytes
    # are 0x16 0x03. The legacy SSLv2 ClientHello heuristic also returns
    # before the non-TLS map. See docs/ops/mtproto-443.md.
    if h[0] == 0x16 and len(h) > 1 and h[1] == 0x03:
        return True
    if (h[0] & 0x80) and len(h) > 4 and h[2] == 1 and (h[3] == 0 or h[3] == 3):
        return True
    return False


def init_header(mode, dc, prefix, avoid_preread_wait):
    forbidden = {b'HEAD', b'POST', b'GET ', b'OPTI', b'\xee'*4,
                 b'\xdd'*4, bytes.fromhex('16030102')}
    if len(prefix) > 8:
        raise ValueError('prefix must be <=8 bytes')
    if prefix and avoid_preread_wait and looks_like_preread_wait(prefix + bytes(8)):
        raise ValueError('prefix forces an ssl_preread wait')
    while True:
        h = bytearray(secrets.token_bytes(64))
        h[:len(prefix)] = prefix
        if h[0] != 0xef and bytes(h[:4]) not in forbidden and h[4:8] != bytes(4):
            if not avoid_preread_wait or not looks_like_preread_wait(h):
                break
        if (len(prefix) >= 1 and prefix[0] == 0xef) or (len(prefix) >= 4 and prefix[:4] in forbidden):
            raise ValueError('prefix conflicts with obfuscated2 blacklist')
        if len(prefix) >= 8 and prefix[4:8] == bytes(4):
            raise ValueError('prefix second word must be nonzero')
    magic = {'abridged': 0xef, 'intermediate': 0xee, 'padded': 0xdd}[mode]
    h[56:60] = bytes([magic])*4
    h[60:62] = struct.pack('<h', dc)
    return bytes(h)


def tl_bytes(data, off):
    n = data[off]
    if n == 254:
        n = int.from_bytes(data[off+1:off+4], 'little')
        start = off+4
    elif n < 254:
        start = off+1
    else:
        raise ValueError('invalid TL bytes length')
    end = start+n
    if end > len(data):
        raise ValueError('short TL bytes')
    return data[start:end], off+((end-off+3)//4)*4


def proxy_v1(src, host, port):
    # TCP4 only. dest is the dial address. Not a credential.
    return ('PROXY TCP4 %s %s 12345 %d\r\n' % (src, host, port)).encode()


def probe(a):
    t = time.monotonic()
    h = init_header(a.mode, a.dc, bytes.fromhex(a.init_prefix), a.avoid_preread_wait)
    nonce = secrets.token_bytes(16)
    body = struct.pack('<I', 0xbe7e8ef1)+nonce
    msg_id = (time.time_ns()*(1<<32)//1_000_000_000) & ~3
    msg = struct.pack('<QQI', 0, msg_id, len(body))+body
    if a.mode == 'abridged':
        framed = bytes([len(msg)//4])+msg
    else:
        pad = secrets.token_bytes(a.padding_bytes) if a.mode == 'padded' else b''
        framed = struct.pack('<I', len(msg)+len(pad))+msg+pad
    enc = ctr(h+framed, h[8:40], h[40:56])
    wire = h[:56]+enc[56:64]+enc[64:]
    if a.proxy_v1_src:
        wire = proxy_v1(a.proxy_v1_src, a.host, a.port)+wire
    reverse = h[8:56][::-1]
    received = bytearray()
    deadline = time.monotonic()+a.timeout
    with socket.create_connection((a.host, a.port), timeout=a.timeout) as s:
        s.sendall(wire)
        while True:
            remaining = deadline-time.monotonic()
            if remaining <= 0:
                raise TimeoutError('total response deadline exceeded')
            s.settimeout(remaining)
            chunk = s.recv(4096)
            if not chunk:
                raise ValueError('EOF before resPQ')
            received.extend(chunk)
            if len(received) > 65536:
                raise ValueError('oversized response')
            raw = ctr(bytes(received), reverse[:32], reverse[32:48])
            if a.mode == 'abridged':
                if raw[0] == 0x7f:
                    if len(raw) < 4:
                        continue
                    n, off = int.from_bytes(raw[1:4], 'little')*4, 4
                else:
                    n, off = raw[0]*4, 1
            else:
                if len(raw) < 4:
                    continue
                n, off = struct.unpack_from('<I', raw)[0], 4
            if not 20 <= n <= 65536:
                raise ValueError('invalid response frame length')
            if len(raw) >= n+off:
                response = raw[off:off+n]
                break
    auth, response_id, body_len = struct.unpack_from('<QQI', response)
    if auth != 0 or body_len < 56 or body_len+20 > len(response):
        raise ValueError('invalid unauthenticated MTProto envelope')
    body = response[20:20+body_len]
    constructor = struct.unpack_from('<I', body)[0]
    if constructor != 0x05162463 or body[4:20] != nonce:
        raise ValueError('not resPQ or nonce mismatch')
    pq, off = tl_bytes(body, 36)
    vector, count = struct.unpack_from('<II', body, off)
    if vector != 0x1cb5c415 or count < 1 or off+8+8*count != len(body):
        raise ValueError('invalid resPQ fingerprint vector')
    return {'ok': True, 'host': a.host, 'port': a.port, 'transport': 'obfuscated2/'+a.mode,
            'request': 'req_pq_multi#be7e8ef1', 'response': 'resPQ#05162463',
            'nonce_match': True, 'response_body_bytes': body_len, 'pq_bytes': len(pq),
            'rsa_fingerprints_count': count, 'response_msg_id_mod4': response_id%4,
            'init_prefix': a.init_prefix or 'random', 'padding_bytes': a.padding_bytes if a.mode == 'padded' else 0,
            'proxy_v1_src': a.proxy_v1_src or '',
            'sni': '',
            'elapsed_ms': round((time.monotonic()-t)*1000, 2)}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('host')
    p.add_argument('port', type=int)
    p.add_argument('--timeout', type=float, default=8)
    p.add_argument('--dc', type=int, default=1)
    p.add_argument('--mode', choices=['abridged', 'intermediate', 'padded'], default='abridged')
    p.add_argument('--padding-bytes', type=int, choices=range(16), default=0, help='padded mode random suffix length, 0..15')
    p.add_argument('--init-prefix', default='', help='hex first bytes for demux edge-case tests')
    p.add_argument('--proxy-v1-src', default='', help='prepend a PROXY v1 TCP4 header with this source IPv4')
    p.add_argument('--avoid-preread-wait', action='store_true',
                   help='reject inits that make nginx ssl_preread wait (TLS record or SSLv2 heuristic)')
    a = p.parse_args()
    started = time.monotonic()
    try:
        result = probe(a)
    except Exception as e:
        result = {'ok': False, 'host': a.host, 'port': a.port, 'transport': 'obfuscated2/'+a.mode,
                  'init_prefix': a.init_prefix or 'random', 'padding_bytes': a.padding_bytes if a.mode == 'padded' else 0,
                  'proxy_v1_src': a.proxy_v1_src or '', 'sni': '',
                  'elapsed_ms': round((time.monotonic()-started)*1000, 2),
                  'error': type(e).__name__+': '+str(e)}
    result['time_cst'] = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat()
    print(json.dumps(result, ensure_ascii=False))
    return 0 if result['ok'] else 1

if __name__ == '__main__':
    sys.exit(main())
