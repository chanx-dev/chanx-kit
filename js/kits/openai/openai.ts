/** The only rate OpenAI realtime takes and its speech produces, as PCM16 mono. */
export const PCM_RATE = 24000;

/** The key given, or OPENAI_API_KEY when used, so a .env loaded after start works. */
export const keyOf = (apiKey?: string) => apiKey || process.env.OPENAI_API_KEY || undefined;
