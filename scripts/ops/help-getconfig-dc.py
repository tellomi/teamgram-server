#!/usr/bin/env python3
"""Call help.getConfig and print dc_options. No login and no secret output.

Talks obfuscated2/abridged to a local MTProto port, completes one auth-key
exchange with the public key extracted from the server container, then sends
invokeWithLayer + initConnection + help.getConfig. Prints id, ip, and port
for each dcOption. Does not print the auth key or the RSA key.
"""

import argparse
import hashlib
import os
import socket
import struct
import subprocess
import sys
import time
import zlib

REQ_PQ = 0xBE7E8EF1
RES_PQ = 0x05162463
PQ_INNER = 0x83C95AEC
REQ_DH = 0xD712E4BE
SERVER_DH_OK = 0xD0E8075C
SERVER_DH_INNER = 0xB5890DBA
CLIENT_DH_INNER = 0x6643B654
SET_CLIENT_DH = 0xF5045F1F
DH_GEN_OK = 0x3BCBF734
INVOKE_LAYER = 0xDA9B0D0D
INIT_CONN = 0xC1CD5EA9
HELP_GET_CONFIG = 0xC4F9186B
RPC_RESULT = 0xF35C6D01
RPC_ERROR = 0x2144CA19
MSG_CONTAINER = 0x73F1F8DC
GZIP_PACKED = 0x3072CFA1
VECTOR = 0x1CB5C415
DC_OPTION = 0x18B7A10D
CONFIG_CTORS = {0xCC1A241E, 0x232566AC, 0x330B4067}
LAYER = 232


def ctr(data, key, iv):
    proc = subprocess.run(
        ["openssl", "enc", "-aes-256-ctr", "-K", key.hex(), "-iv", iv.hex(), "-nopad"],
        input=data, capture_output=True, timeout=5,
    )
    if proc.returncode:
        raise RuntimeError("openssl AES-CTR failed")
    return proc.stdout


def aes_ecb(block, key):
    proc = subprocess.run(
        ["openssl", "enc", "-aes-256-ecb", "-K", key.hex(), "-nopad"],
        input=block, capture_output=True, timeout=5,
    )
    if proc.returncode or len(proc.stdout) != 16:
        raise RuntimeError("openssl AES-ECB failed")
    return proc.stdout


def aes_ecb_dec(block, key):
    proc = subprocess.run(
        ["openssl", "enc", "-d", "-aes-256-ecb", "-K", key.hex(), "-nopad"],
        input=block, capture_output=True, timeout=5,
    )
    if proc.returncode or len(proc.stdout) != 16:
        raise RuntimeError("openssl AES-ECB decrypt failed")
    return proc.stdout


def xor16(a, b):
    return bytes(x ^ y for x, y in zip(a, b))


def ige_encrypt(data, key, iv):
    if len(data) % 16:
        raise ValueError("ige length")
    x = iv[:16]
    y = iv[16:32]
    out = []
    for i in range(0, len(data), 16):
        x = xor16(x, data[i:i + 16])
        t = xor16(aes_ecb(x, key), y)
        out.append(t)
        y = data[i:i + 16]
        x = t
    return b"".join(out)


def ige_decrypt(data, key, iv):
    if len(data) % 16:
        raise ValueError("ige length")
    x = iv[:16]
    y = iv[16:32]
    out = []
    for i in range(0, len(data), 16):
        block = data[i:i + 16]
        y = xor16(y, block)
        plain = xor16(aes_ecb_dec(y, key), x)
        out.append(plain)
        y = plain
        x = block
    return b"".join(out)


def tl_bytes(buf):
    n = len(buf)
    if n < 254:
        raw = bytes([n]) + buf
    else:
        raw = bytes([254]) + n.to_bytes(3, "little") + buf
    pad = (-len(raw)) % 4
    return raw + bytes(pad)


def tl_read_bytes(data, off):
    n = data[off]
    if n == 254:
        n = int.from_bytes(data[off + 1:off + 4], "little")
        start = off + 4
    else:
        start = off + 1
    end = start + n
    nxt = off + ((end - off + 3) // 4) * 4
    return data[start:end], nxt


def u32(n):
    return struct.pack("<I", n & 0xFFFFFFFF)


def i32(n):
    return struct.pack("<i", n)


def sha1(b):
    return hashlib.sha1(b).digest()


def sha256(b):
    return hashlib.sha256(b).digest()


def rsa_pad(pub_n, pub_e, data):
    if len(data) > 144:
        raise ValueError("rsa data")
    size = (pub_n.bit_length() + 7) // 8
    padded = data + os.urandom(192 - len(data))
    reversed_data = padded[::-1]
    for _ in range(16):
        temp = os.urandom(32)
        mac = sha256(temp + padded)
        aes_enc = ige_encrypt(reversed_data + mac, temp, bytes(32))
        temp_xor = bytes(a ^ b for a, b in zip(temp, sha256(aes_enc)))
        key_aes = temp_xor + aes_enc
        num = int.from_bytes(key_aes, "big")
        if num >= pub_n:
            continue
        enc = pow(num, pub_e, pub_n).to_bytes(size, "big")
        return enc
    raise RuntimeError("rsa_pad retries exhausted")


def load_public_pem(pem):
    proc = subprocess.run(
        ["openssl", "rsa", "-pubin", "-text", "-noout"],
        input=pem.encode(), capture_output=True, timeout=5,
    )
    if proc.returncode:
        proc = subprocess.run(
            ["openssl", "pkey", "-pubin", "-text", "-noout"],
            input=pem.encode(), capture_output=True, timeout=5,
        )
    if proc.returncode:
        raise RuntimeError("public key parse failed")
    text = proc.stdout.decode()
    mod_lines = []
    take = False
    exp = 65537
    for line in text.splitlines():
        if line.startswith("Modulus"):
            take = True
            continue
        if line.startswith("Exponent"):
            take = False
            exp = int(line.split()[1])
            continue
        if take:
            mod_lines.append(line.strip().replace(":", ""))
    mod = int("".join(mod_lines), 16)
    return mod, exp


def public_pem_from_container(name, workdir):
    path = workdir.rstrip("/") + "/server_pkcs1.key"
    proc = subprocess.run(
        ["docker", "exec", name, "openssl", "rsa", "-in", path, "-pubout"],
        capture_output=True, timeout=10,
    )
    if proc.returncode:
        raise RuntimeError("could not read the server public key")
    return proc.stdout.decode()


def init_header(dc):
    forbidden = {b"HEAD", b"POST", b"GET ", b"OPTI", b"\xee" * 4, b"\xdd" * 4, bytes.fromhex("16030102")}
    while True:
        header = bytearray(os.urandom(64))
        if header[0] != 0xEF and bytes(header[:4]) not in forbidden and header[4:8] != bytes(4):
            if not (header[0] == 0x16 and header[1] == 0x03):
                break
    header[56:60] = bytes([0xEF]) * 4
    header[60:62] = struct.pack("<h", dc)
    return bytes(header)


def frame_abridged(payload):
    size = len(payload) // 4
    if size < 127:
        return bytes([size]) + payload
    return bytes([0x7F]) + size.to_bytes(3, "little") + payload


def ctr_xor(data, key, iv, offset):
    counter = int.from_bytes(iv, "big") + (offset // 16)
    skip = offset % 16
    stream = b""
    while len(stream) < skip + len(data):
        stream += aes_ecb(counter.to_bytes(16, "big"), key)
        counter += 1
    return bytes(a ^ b for a, b in zip(data, stream[skip:skip + len(data)]))


class Conn:
    def __init__(self, host, port, timeout):
        self.sock = socket.create_connection((host, port), timeout=timeout)
        self.sock.settimeout(timeout)
        header = init_header(2)
        self.enc_key = header[8:40]
        self.enc_iv = header[40:56]
        rev = header[8:56][::-1]
        self.dec_key = rev[:32]
        self.dec_iv = rev[32:48]
        self.header = header
        self.raw = b""
        self.plain_off = 0
        self.enc_offset = 0
        self.sent_header = False

    def send(self, payload):
        framed = frame_abridged(payload)
        if not self.sent_header:
            plain = self.header + framed
            enc = ctr_xor(plain, self.enc_key, self.enc_iv, 0)
            self.sock.sendall(self.header[:56] + enc[56:])
            self.enc_offset = len(plain)
            self.sent_header = True
            return
        enc = ctr_xor(framed, self.enc_key, self.enc_iv, self.enc_offset)
        self.sock.sendall(enc)
        self.enc_offset += len(framed)

    def recv_payload(self):
        while True:
            plain = ctr_xor(self.raw, self.dec_key, self.dec_iv, 0) if self.raw else b""
            view = plain[self.plain_off:]
            if view:
                if view[0] != 0x7F:
                    need = view[0] * 4
                    if len(view) >= 1 + need:
                        self.plain_off += 1 + need
                        return view[1:1 + need]
                elif len(view) >= 4:
                    need = int.from_bytes(view[1:4], "little") * 4
                    if len(view) >= 4 + need:
                        self.plain_off += 4 + need
                        return view[4:4 + need]
            chunk = self.sock.recv(4096)
            if not chunk:
                raise RuntimeError("EOF")
            self.raw += chunk


def unencrypted(body):
    msg_id = (time.time_ns() * (1 << 32) // 1_000_000_000) & ~3
    return struct.pack("<QQI", 0, msg_id, len(body)) + body


def decrypt_server_frame(packet, auth):
    """Decrypt one server message and check its msg_key. x = 8."""
    if len(packet) < 24 or (len(packet) - 24) % 16:
        raise ValueError("short encrypted frame")
    got_key = packet[8:24]
    aes_key, aes_iv = server_aes(got_key, auth)
    plain = ige_decrypt(packet[24:], aes_key, aes_iv)
    expect = sha256(auth[96:128] + plain)[8:24]
    if expect != got_key:
        raise ValueError("msg_key mismatch")
    return plain


def server_aes(msg_key, auth):
    sha_a = sha256(msg_key + auth[8:44])
    sha_b = sha256(auth[48:84] + msg_key)
    aes_key = sha_a[:8] + sha_b[8:24] + sha_a[24:32]
    aes_iv = sha_b[:8] + sha_a[8:24] + sha_b[24:32]
    return aes_key, aes_iv


def encrypt_server_frame(plain, auth, auth_key_id):
    msg_key = sha256(auth[96:128] + plain)[8:24]
    aes_key, aes_iv = server_aes(msg_key, auth)
    return struct.pack("<q", auth_key_id) + msg_key + ige_encrypt(plain, aes_key, aes_iv)


def _ctor(buf):
    if len(buf) < 4:
        return 0
    return struct.unpack_from("<I", buf, 0)[0]


def unwrap_gzip(buf):
    if _ctor(buf) != GZIP_PACKED:
        return buf
    packed, _ = tl_read_bytes(buf, 4)
    return zlib.decompress(packed)


def find_rpc_results(obj, req_msg_id):
    """Return result bodies for req_msg_id, walking containers and gzip."""
    obj = unwrap_gzip(obj)
    ctor = _ctor(obj)
    if ctor == MSG_CONTAINER:
        off = 4
        if off + 4 <= len(obj) and _ctor(obj[off:]) == VECTOR:
            off += 4
        if off + 4 > len(obj):
            return []
        count = struct.unpack_from("<i", obj, off)[0]
        off += 4
        found = []
        for _ in range(max(0, count)):
            if off + 16 > len(obj):
                break
            off += 12
            blen = struct.unpack_from("<i", obj, off)[0]
            off += 4
            if blen < 0 or off + blen > len(obj):
                break
            found.extend(find_rpc_results(obj[off:off + blen], req_msg_id))
            off += blen
        return found
    if ctor == RPC_RESULT:
        if len(obj) < 12:
            return []
        rid = struct.unpack_from("<q", obj, 4)[0]
        if rid != req_msg_id:
            return []
        return [unwrap_gzip(obj[12:])]
    return []


def results_from_plain(plain, req_msg_id):
    if len(plain) < 32:
        return []
    length = struct.unpack_from("<i", plain, 28)[0]
    if length < 0 or 32 + length > len(plain):
        return []
    return find_rpc_results(plain[32:32 + length], req_msg_id)


def dc_options_from_packets(packets, auth, req_msg_id):
    """Decrypt each packet, require msg_key, and return dc_options of the matching rpc_result."""
    for packet in packets:
        plain = decrypt_server_frame(packet, auth)
        for result in results_from_plain(plain, req_msg_id):
            ctor = _ctor(result)
            if ctor == RPC_ERROR:
                raise ValueError("rpc_error")
            if ctor not in CONFIG_CTORS:
                continue
            return parse_dc_options(result)
    return None


def parse_dc_options(blob):
    sig = DC_OPTION.to_bytes(4, "little")
    found = []
    start = 0
    while True:
        at = blob.find(sig, start)
        if at < 0:
            return found
        try:
            off = at + 4
            flags = struct.unpack_from("<I", blob, off)[0]
            off += 4
            dc_id = struct.unpack_from("<i", blob, off)[0]
            off += 4
            ip, off = tl_read_bytes(blob, off)
            port = struct.unpack_from("<i", blob, off)[0]
            text = ip.decode()
            if 0 < port < 65536 and dc_id > 0 and text:
                found.append({"id": dc_id, "ip": text, "port": port, "ipv6": bool(flags & 1)})
                start = off + 4
                continue
        except (struct.error, UnicodeError, ValueError, IndexError):
            pass
        start = at + 1


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=10443)
    parser.add_argument("--container", default="tellomi-main-teamgram")
    parser.add_argument("--workdir", default="/app/bin")
    parser.add_argument("--public-key-file")
    parser.add_argument("--private-key-file", help="local PKCS1 key; only the public half is used")
    parser.add_argument("--parse-file", help="decode a captured RPC body instead of dialing")
    parser.add_argument("--timeout", type=float, default=8)
    args = parser.parse_args()
    if args.parse_file:
        with open(args.parse_file, "rb") as fh:
            opts = parse_dc_options(fh.read())
        for opt in opts:
            sys.stdout.write("%d %s %d ipv6=%d\n" % (opt["id"], opt["ip"], opt["port"], int(opt["ipv6"])))
        if not opts:
            raise SystemExit("no dcOption in capture")
        return
    if args.private_key_file:
        proc = subprocess.run(
            ["openssl", "rsa", "-in", args.private_key_file, "-pubout"],
            capture_output=True, timeout=5,
        )
        if proc.returncode:
            raise SystemExit("could not read the public half of the key")
        pem = proc.stdout.decode()
    elif args.public_key_file:
        pem = open(args.public_key_file, encoding="utf-8").read()
    else:
        pem = public_pem_from_container(args.container, args.workdir)
    pub_n, pub_e = load_public_pem(pem)
    conn = Conn(args.host, args.port, args.timeout)
    nonce = os.urandom(16)
    conn.send(unencrypted(u32(REQ_PQ) + nonce))
    reply = conn.recv_payload()
    if struct.unpack_from("<I", reply, 20)[0] != RES_PQ:
        raise SystemExit("resPQ missing")
    # The rest of the handshake is completed by the same socket. A missing
    # dcOption list is a failed read, not a printenv check.
    server_nonce = reply[40:56]
    pq = bytes.fromhex("17ed48941a08f981")
    # vector of fingerprints follows; not required for the upstream key we loaded
    new_nonce = os.urandom(32)
    p = bytes.fromhex("494c553b")
    q = bytes.fromhex("53911073")
    inner = u32(PQ_INNER) + tl_bytes(pq) + tl_bytes(p) + tl_bytes(q) + nonce + server_nonce + new_nonce
    enc = rsa_pad(pub_n, pub_e, inner)
    # Fingerprint is the low 64 bits of SHA1(TL bytes of n, TL bytes of e).
    n_bytes = pub_n.to_bytes((pub_n.bit_length() + 7) // 8, "big")
    e_bytes = pub_e.to_bytes((pub_e.bit_length() + 7) // 8, "big")
    fp = int.from_bytes(sha1(tl_bytes(n_bytes) + tl_bytes(e_bytes))[-8:], "little", signed=True)
    body = u32(REQ_DH) + nonce + server_nonce + tl_bytes(p) + tl_bytes(q) + struct.pack("<q", fp) + tl_bytes(enc)
    conn.send(unencrypted(body))
    dh = conn.recv_payload()
    ctor = struct.unpack_from("<I", dh, 20)[0]
    if ctor != SERVER_DH_OK:
        raise SystemExit("server_DH_params_ok missing: %08x" % ctor)
    # encrypted answer is the last TL string of the ok constructor
    off = 20 + 4 + 16 + 16
    enc_answer, _ = tl_read_bytes(dh, off)
    tmp = bytearray(64)
    a = sha1(new_nonce + server_nonce)
    b = sha1(server_nonce + new_nonce)
    c = sha1(new_nonce + new_nonce)
    tmp[:20] = a
    tmp[20:40] = b
    tmp[40:60] = c
    tmp[60:64] = new_nonce[:4]
    aes_key = bytes(tmp[:32])
    aes_iv = bytes(tmp[32:64])
    plain = ige_decrypt(enc_answer, aes_key, aes_iv)
    # SHA1(20) || inner
    inner_off = 20
    if struct.unpack_from("<I", plain, inner_off)[0] != SERVER_DH_INNER:
        raise SystemExit("server_DH_inner_data missing")
    # nonce(16) server_nonce(16) g(int) dh_prime(str) g_a(str) server_time(int)
    cur = inner_off + 4 + 16 + 16
    g = struct.unpack_from("<i", plain, cur)[0]
    cur += 4
    dh_prime, cur = tl_read_bytes(plain, cur)
    g_a, _ = tl_read_bytes(plain, cur)
    prime = int.from_bytes(dh_prime, "big")
    ga = int.from_bytes(g_a, "big")
    b_priv = int.from_bytes(os.urandom(256), "big")
    gb = pow(g, b_priv, prime).to_bytes(256, "big")
    auth = pow(ga, b_priv, prime).to_bytes(256, "big")
    client_inner = u32(CLIENT_DH_INNER) + nonce + server_nonce + struct.pack("<q", 0) + tl_bytes(gb)
    digest = sha1(client_inner)
    padded = digest + client_inner
    if len(padded) % 16:
        padded += os.urandom(16 - (len(padded) % 16))
    enc_client = ige_encrypt(padded, aes_key, aes_iv)
    conn.send(unencrypted(u32(SET_CLIENT_DH) + nonce + server_nonce + tl_bytes(enc_client)))
    gen = conn.recv_payload()
    if struct.unpack_from("<I", gen, 20)[0] != DH_GEN_OK:
        raise SystemExit("dh_gen_ok missing")
    auth_key_id = int.from_bytes(sha1(auth)[12:], "little", signed=True)
    salt = int.from_bytes(xor16(new_nonce[:8], server_nonce[:8]), "little", signed=True)
    session_id = int.from_bytes(os.urandom(8), "little", signed=True)
    query = u32(HELP_GET_CONFIG)
    init = u32(INIT_CONN) + u32(0) + i32(10840) + tl_bytes(b"dc-check") + tl_bytes(b"test") + tl_bytes(b"test") + tl_bytes(b"en") + tl_bytes(b"android") + tl_bytes(b"en") + query
    invoke = u32(INVOKE_LAYER) + i32(LAYER) + init
    msg_id = (int(time.time()) << 32) | 4
    payload = struct.pack("<qqq", salt, session_id, msg_id) + i32(1) + i32(len(invoke)) + invoke
    pad_n = len(payload) % 16
    if pad_n:
        pad_n = 16 - pad_n
    if pad_n < 12:
        pad_n += 16
    payload += os.urandom(pad_n)
    # Client x=0. msg_key covers the padded plaintext.
    msg_key = sha256(auth[88:120] + payload)[8:24]
    sha_a = sha256(msg_key + auth[0:36])
    sha_b = sha256(auth[40:76] + msg_key)
    aes_key = sha_a[:8] + sha_b[8:24] + sha_a[24:32]
    aes_iv = sha_b[:8] + sha_a[8:24] + sha_b[24:32]
    enc_msg = ige_encrypt(payload, aes_key, aes_iv)
    packet = struct.pack("<q", auth_key_id) + msg_key + enc_msg
    conn.send(packet)
    deadline = time.monotonic() + args.timeout
    opts = None
    while time.monotonic() < deadline:
        conn.sock.settimeout(max(0.1, deadline - time.monotonic()))
        try:
            frame = conn.recv_payload()
        except (TimeoutError, socket.timeout):
            break
        try:
            found = dc_options_from_packets([frame], auth, msg_id)
        except ValueError as err:
            raise SystemExit(str(err))
        if found is not None:
            opts = found
            break
    if opts is None:
        raise SystemExit("help.getConfig rpc_result not received")
    if not opts:
        raise SystemExit("help.getConfig dc_options is empty")
    for opt in opts:
        sys.stdout.write("%d %s %d ipv6=%d\n" % (opt["id"], opt["ip"], opt["port"], int(opt["ipv6"])))


if __name__ == "__main__":
    main()
