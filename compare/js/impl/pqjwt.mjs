// @noble/post-quantum's version of ../../pqjwt/pqjwt.go.
import { ml_dsa65 } from "@noble/post-quantum/ml-dsa.js";

const enc = new TextEncoder();
const dec = new TextDecoder();

function b64(bytes) {
  let s = "";
  for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function unb64(s) {
  if (!/^[A-Za-z0-9_-]*$/.test(s) || s.length % 4 === 1) throw new Error("pqjwt: invalid base64url");
  const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
  const b = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) b[i] = bin.charCodeAt(i);
  return b;
}

function hexBytes(h) {
  const b = new Uint8Array(h.length / 2);
  for (let i = 0; i < b.length; i++) b[i] = parseInt(h.substr(i * 2, 2), 16);
  return b;
}

const keys = new Map();
function key(seedHex) {
  let k = keys.get(seedHex);
  if (!k) keys.set(seedHex, (k = ml_dsa65.keygen(hexBytes(seedHex))));
  return k;
}

export function PublicKey(seedHex) {
  return b64(key(seedHex).publicKey);
}

export function Issue(seedHex, kid, sub, exp) {
  const k = key(seedHex);
  const input = b64(enc.encode(JSON.stringify({ alg: "ML-DSA-65", typ: "JWT", kid }))) + "." + b64(enc.encode(JSON.stringify({ sub, iss: "https://id.example.com", exp })));
  const sig = ml_dsa65.sign(enc.encode(input), k.secretKey, { extraEntropy: false });
  return input + "." + b64(sig);
}

const pubs = new Map();
export function Verify(token, pub, now) {
  let pk = pubs.get(pub);
  if (!pk) pubs.set(pub, (pk = unb64(pub)));
  const parts = token.split(".");
  if (parts.length !== 3) throw new Error("pqjwt: not a JWS");
  const h = JSON.parse(dec.decode(unb64(parts[0])));
  if (h.alg !== "ML-DSA-65") throw new Error("pqjwt: unexpected alg " + h.alg);
  if (!ml_dsa65.verify(unb64(parts[2]), enc.encode(parts[0] + "." + parts[1]), pk)) throw new Error("pqjwt: invalid signature");
  const c = JSON.parse(dec.decode(unb64(parts[1])));
  if (c.exp <= now) throw new Error("pqjwt: token expired");
  return c.sub;
}
