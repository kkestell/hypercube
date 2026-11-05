// The page the desktop app shows until the Hypercube service answers: where
// the app looks for it, how to start it, and, when the app may install the
// login service, the button that does.
import {useEffect, useState, type ReactNode} from 'react';
import {createRoot} from 'react-dom/client';
import {startAppearance} from '../app/appearance.ts';
import {Alert, AlertDescription} from '../components/ui/alert';
import {Button} from '../components/ui/button';

const bridge = globalThis.hypercubeSetup!;

const Code = ({children}: {children: ReactNode}) => <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm break-all">{children}</code>;

function Guidance({status, setUp, busy}: {status: SetupStatus; setUp: () => void; busy: boolean}) {
  switch (status.action) {
    case 'headless':
      return <p>Start it with <Code>{`hypercube ui ${status.port}`}</Code>.</p>;
    case 'kickstart':
      return <p>The login service is loaded but not answering. Start it with <Code>{`launchctl kickstart gui/${status.uid}/${status.service}`}</Code>.</p>;
    case 'move':
      return <p>This app is running from a temporary location. Move Hypercube to a folder such as <Code>~/Applications</Code> and open it from there to set up the service.</p>;
    case 'setup':
      return <>
        <p>Set Up Background Service starts Hypercube at login from <Code>{status.bundle}</Code>. This app does not start at login; add it in System Settings → General → Login Items if you want that.</p>
        <div><Button disabled={busy} onClick={setUp}>Set Up Background Service</Button></div>
      </>;
  }
}

function Waiting() {
  const [status, setStatus] = useState<SetupStatus | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => { bridge.status().then(setStatus); }, []);
  const setUp = async () => {
    setBusy(true);
    setError(await bridge.install());
    setStatus(await bridge.status());
    setBusy(false);
  };
  if (!status) return null;
  return <main className="mx-auto flex h-full max-w-xl flex-col justify-center gap-4 p-8">
    <div className="fixed inset-x-0 top-0 h-12 [app-region:drag]" />
    <h1 className="text-xl font-semibold">Waiting for Hypercube</h1>
    <p>The Hypercube service is not answering at <Code>{status.address}</Code>. Its log is <Code>{status.log}</Code>.</p>
    <Guidance status={status} setUp={setUp} busy={busy} />
    {error && <Alert variant="destructive"><AlertDescription className="whitespace-pre-wrap">{error}</AlertDescription></Alert>}
  </main>;
}

startAppearance();
createRoot(document.getElementById('app')!).render(<Waiting />);
