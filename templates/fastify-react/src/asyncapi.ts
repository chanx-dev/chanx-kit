// Prints the AsyncAPI document, for npm --prefix web run gen.
import { asyncapi, chanx } from './app';

process.stdout.write(JSON.stringify(chanx.asyncapi(asyncapi), null, 2));
process.exit(0);
