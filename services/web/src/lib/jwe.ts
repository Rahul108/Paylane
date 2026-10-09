import * as jose from 'jose';
import fs from 'fs';
import path from 'path';

function getKeysDir(): string {
  if (process.env.KEYS_DIR) return process.env.KEYS_DIR;
  if (fs.existsSync('/keys')) return '/keys';
  if (fs.existsSync(path.resolve(process.cwd(), '../../keys'))) return path.resolve(process.cwd(), '../../keys');
  if (fs.existsSync(path.resolve(process.cwd(), 'keys'))) return path.resolve(process.cwd(), 'keys');
  return path.resolve(process.cwd(), '../../keys');
}

export async function sealPayload(
  payload: any,
  receiverService: string,
  senderService: string = 'web',
  ttlSeconds: number = 60
): Promise<string> {
  const keysDir = getKeysDir();
  const signPrivPEM = fs.readFileSync(path.join(keysDir, senderService, 'sign_private.pem'), 'utf8');
  const receiverEncPubPEM = fs.readFileSync(path.join(keysDir, receiverService, 'enc_public.pem'), 'utf8');

  const signKey = await jose.importPKCS8(signPrivPEM, 'RS256');
  const encKey = await jose.importSPKI(receiverEncPubPEM, 'RSA-OAEP-256');

  const now = Math.floor(Date.now() / 1000);
  const jti = `jti_${now}_${Math.random().toString(36).substring(2, 9)}`;

  const nested = {
    claims: {
      iss: senderService,
      aud: receiverService,
      iat: now,
      exp: now + ttlSeconds,
      jti: jti,
    },
    data: Buffer.from(JSON.stringify(payload)),
  };

  // Sign (JWS RS256)
  const jws = await new jose.CompactSign(Buffer.from(JSON.stringify(nested)))
    .setProtectedHeader({ alg: 'RS256', typ: 'JWT' })
    .sign(signKey);

  // Encrypt (JWE RSA-OAEP-256 + A256GCM)
  const jwe = await new jose.CompactEncrypt(new TextEncoder().encode(jws))
    .setProtectedHeader({ alg: 'RSA-OAEP-256', enc: 'A256GCM', typ: 'JWT', cty: 'JWT' })
    .encrypt(encKey);

  return jwe;
}

export async function openPayload(
  token: string,
  expectedAud: string = 'web',
  serviceName: string = 'web'
): Promise<{ payload: any; claims: any }> {
  const keysDir = getKeysDir();
  const encPrivPEM = fs.readFileSync(path.join(keysDir, serviceName, 'enc_private.pem'), 'utf8');
  const encKey = await jose.importPKCS8(encPrivPEM, 'RSA-OAEP-256');

  // Decrypt JWE
  const { plaintext } = await jose.compactDecrypt(token, encKey, {
    keyManagementAlgorithms: ['RSA-OAEP-256'],
    contentEncryptionAlgorithms: ['A256GCM'],
  });

  const jwsStr = new TextDecoder().decode(plaintext);

  // Verify inner JWS
  // Extract unverified header/payload to find issuer
  const unverified = jose.decodeJwt(jwsStr) as any;
  const issuer = unverified?.claims?.iss;
  if (!issuer) throw new Error('missing issuer in token');

  const senderSignPubPEM = fs.readFileSync(path.join(keysDir, issuer, 'sign_public.pem'), 'utf8');
  const signKey = await jose.importSPKI(senderSignPubPEM, 'RS256');

  const { payload: verifiedBuffer } = await jose.compactVerify(jwsStr, signKey, {
    algorithms: ['RS256'],
  });

  const nested = JSON.parse(new TextDecoder().decode(verifiedBuffer));
  if (expectedAud && nested.claims.aud !== expectedAud) {
    throw new Error(`invalid audience: expected ${expectedAud}, got ${nested.claims.aud}`);
  }

  const rawData = nested.data ? Buffer.from(nested.data).toString('utf8') : null;
  return {
    claims: nested.claims,
    payload: rawData ? JSON.parse(rawData) : null,
  };
}
