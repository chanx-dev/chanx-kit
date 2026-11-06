import { useEffect, useState } from 'react';

import { DEMOS, type Sandbox, type Server } from './demos';

const DOCS = 'https://kit.chanx.dev';
const REPO = 'https://github.com/chanx-dev/chanx-kit/blob/main';
const SOURCE: Record<Server, string> = {
  python: `${REPO}/python/sandbox/consumers.py`,
  go: `${REPO}/go/sandbox/consumers.go`,
  js: `${REPO}/js/sandbox/consumers.ts`,
};
const FALLBACK: Sandbox = {
  server: 'python',
  framework: 'fastapi',
  providers: { stt: 'fake-voice', tts: 'fake-voice' },
};

/** The demo in the URL hash (`#/voice-agent`), so each one can be linked to. */
function useDemoId(): string {
  const read = () => window.location.hash.replace(/^#\/?/, '') || DEMOS[0].id;
  const [id, setId] = useState(read);
  useEffect(() => {
    const onChange = () => setId(read());
    window.addEventListener('hashchange', onChange);
    return () => window.removeEventListener('hashchange', onChange);
  }, []);
  return id;
}

function useSandbox(): Sandbox {
  const [sandbox, setSandbox] = useState<Sandbox>(FALLBACK);
  useEffect(() => {
    fetch('/api/sandbox')
      .then((response) => (response.ok ? response.json() : FALLBACK))
      .then(setSandbox)
      .catch(() => setSandbox(FALLBACK));
  }, []);
  return sandbox;
}

export function App() {
  const [who, setWho] = useState('chris');
  const id = useDemoId();
  const sandbox = useSandbox();
  const { providers, server } = sandbox;
  const demos = DEMOS.filter((candidate) => candidate.servers.includes(server));
  const groups = [...new Set(demos.map((candidate) => candidate.group))];
  const demo = demos.find((candidate) => candidate.id === id) ?? DEMOS[0];

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">
          <h1>chanx-kit</h1>
          <p className="hint">
            Sandbox · {server} · {sandbox.framework}
          </p>
        </div>
        <label>
          Connect as
          <input value={who} onChange={(event) => setWho(event.target.value)} />
        </label>
        <nav aria-label="Demos">
          {groups.map((group) => (
            <div key={group} className="nav-group">
              <p className="nav-title">{group}</p>
              {demos.filter((candidate) => candidate.group === group).map((candidate) => (
                <a
                  key={candidate.id}
                  href={`#/${candidate.id}`}
                  aria-current={candidate.id === demo.id ? 'page' : undefined}
                >
                  {candidate.title}
                </a>
              ))}
            </div>
          ))}
        </nav>
        {server === 'python' && (
          <p className="hint">
            Voice: <code>{providers.stt}</code> listens, <code>{providers.tts}</code> speaks.
            Set <code>DEEPGRAM_API_KEY</code> / <code>ELEVENLABS_API_KEY</code> in{' '}
            <code>.env</code> for real ones.
          </p>
        )}
        <p className="hint">
          <a href="/asyncapi">AsyncAPI docs</a> · <a href={`${DOCS}/`}>Kit docs</a>
        </p>
      </aside>

      <main>
        <header className="demo-head">
          <h2>{demo.title}</h2>
          <p>{demo.summary}</p>
          <dl className="demo-meta">
            <dt>Server kits</dt>
            <dd>
              {demo.kits(providers).map((kit) => (
                <a key={kit} className="chip" href={`${DOCS}/kits/${kit}/`}>
                  {kit}
                </a>
              ))}
            </dd>
            <dt>UI kits</dt>
            <dd>
              {demo.ui.map((kit) => (
                <a key={kit} className="chip" href={`${DOCS}/ui/${kit}/`}>
                  {kit}
                </a>
              ))}
            </dd>
            <dt>Consumer</dt>
            <dd>
              <a href={SOURCE[server]}>
                <code>{demo.consumer}</code>
              </a>
            </dd>
          </dl>
        </header>
        {/* Keyed, so leaving a demo closes its socket and frees the microphone. */}
        <div key={demo.id} className="demo">
          {demo.render(who, sandbox)}
        </div>
      </main>
    </div>
  );
}
