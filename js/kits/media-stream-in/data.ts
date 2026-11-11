import { z } from 'zod';

const isBase64 = (value: string) => /^[A-Za-z0-9+/]*={0,2}$/.test(value) && value.length % 4 === 0;

/** Base64 a server sends: checked, and kept as the string the wire carries. */
export const Base64Text = z
  .string()
  .meta({ format: 'base64', contentEncoding: 'base64' })
  .refine(isBase64, 'not valid base64');

/** Bytes a handler receives as a `Buffer`; the wire carries standard base64. */
export const Base64Data = Base64Text.transform((value) => Buffer.from(value, 'base64'));

export const toBase64 = (data: Uint8Array) => Buffer.from(data).toString('base64');
