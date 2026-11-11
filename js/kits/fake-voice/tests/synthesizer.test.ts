import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { describe, expect, it } from 'vitest';

import { fakeSynthesizerTopic, wordTones } from '../synthesizer';

describe('fake synthesizer', () => {
  it('makes one tone per word at the requested rate', () => {
    const pieces = wordTones('one two', { encoding: 'pcm16', sample_rate: 16000, channels: 1 }, 100, 50);
    expect(pieces.map((p) => p.length)).toEqual([(1600 + 800) * 2, (1600 + 800) * 2]);
  });

  it('doubles the frames in stereo', () => {
    const mono = wordTones('hi', { encoding: 'pcm16', sample_rate: 16000, channels: 1 }, 100, 50);
    const stereo = wordTones('hi', { encoding: 'pcm16', sample_rate: 16000, channels: 2 }, 100, 50);
    expect(stereo[0]!.length).toBe(2 * mono[0]!.length);
  });

  it('speaks text through the topic', async () => {
    const topic = fakeSynthesizerTopic({ pace: 0 });
    const chanx = createChanx({ consumers: [consumer('/ws', { name: 'voice', topics: [topic] })] });
    const c = connect(chanx, '/ws');
    await c.subscribe('speak:fake');
    c.send({ action: 'speak', topic: 'speak:fake', payload: { text: 'two words' } });
    const end = (await c.receiveUntil('audio_end')).at(-1)!;
    expect((end.payload as { duration_ms: number }).duration_ms).toBe(500);
  });
});
