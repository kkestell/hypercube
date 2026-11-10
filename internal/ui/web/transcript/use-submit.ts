// useSubmit runs a send and reports whether it is under way. A send already
// under way ignores another, and a refused send enables the controls again.
import {useState} from 'react';

export function useSubmit() {
  const [sending, setSending] = useState(false);
  const submit = async (send: () => Promise<boolean>) => {
    if (sending) return;
    setSending(true);
    if (!await send()) setSending(false);
  };
  return [sending, submit] as const;
}
