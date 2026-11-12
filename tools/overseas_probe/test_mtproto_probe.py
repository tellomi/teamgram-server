import argparse
import socket
import struct
import unittest
from unittest.mock import patch
import mtproto_probe as m


class Socket:
    def __init__(self, chunks):
        self.chunks = iter(chunks)
    def __enter__(self):
        return self
    def __exit__(self, *args):
        pass
    def sendall(self, wire):
        pass
    def settimeout(self, value):
        pass
    def recv(self, size):
        value = next(self.chunks, b'')
        if isinstance(value, Exception):
            raise value
        return value


class ProbeTests(unittest.TestCase):
    def args(self):
        return argparse.Namespace(host='example.invalid', port=443, dc=2,
            mode='abridged', init_prefix='', timeout=1, padding_bytes=0)

    def failure(self, chunks):
        with patch.object(m.socket, 'create_connection', return_value=Socket(chunks)):
            with self.assertRaises(m.ProbeFailure) as cm:
                m.probe(self.args())
        return cm.exception

    def test_http_failure_preserves_only_received_prefix(self):
        raw = b'HTTP/1.1 400 Bad Request\r\nServer: Pingora\r\n\r\n'
        e = self.failure([raw])
        self.assertEqual(e.raw_class, 'http')
        self.assertEqual(e.raw_first_bytes_hex, raw.hex())
        self.assertEqual(e.received_bytes, len(raw))

    def test_tls_alert(self):
        e = self.failure([b'\x15\x03\x03\x00\x02\x02\x28'])
        self.assertEqual(e.raw_class, 'tls_alert')

    def test_no_response(self):
        e = self.failure([b''])
        self.assertEqual(e.raw_first_bytes_hex, '')
        self.assertEqual(e.raw_class, 'no_response')

    def test_timeout_keeps_partial_response(self):
        header = bytes(range(64))
        reverse = header[8:56][::-1]
        wire = m.ctr(b'\x7f', reverse[:32], reverse[32:48])
        with patch.object(m, 'init_header', return_value=header):
            e = self.failure([wire, socket.timeout('test timeout')])
        self.assertIn('timeout', str(e))
        self.assertEqual(e.raw_first_bytes_hex, wire.hex())
        self.assertEqual(e.received_bytes, 1)

    def test_valid_res_pq(self):
        header, nonce = bytes(range(64)), b'N'*16
        body = (struct.pack('<I', 0x05162463) + nonce + b'S'*16 +
                b'\x08'+b'P'*8+b'\0'*3 + struct.pack('<IIQ', 0x1cb5c415, 1, 1))
        envelope = struct.pack('<QQI', 0, 1, len(body)) + body
        frame = bytes([len(envelope)//4])+envelope
        reverse = header[8:56][::-1]
        wire = m.ctr(frame, reverse[:32], reverse[32:48])
        with patch.object(m, 'init_header', return_value=header), \
             patch.object(m.secrets, 'token_bytes', return_value=nonce), \
             patch.object(m.socket, 'create_connection', return_value=Socket([wire])):
            result = m.probe(self.args())
        self.assertTrue(result['ok'])
        self.assertTrue(result['nonce_match'])
        self.assertEqual(result['response'], 'resPQ#05162463')


if __name__ == '__main__':
    unittest.main()
