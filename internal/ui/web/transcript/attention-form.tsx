import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {Label} from '../components/ui/label';
// The answers to an open attention request: content-sized choice buttons
// with their notes, and a labeled reply field with a separate Send button
// when the request takes typed text. The transcript and notification cards
// both draw it, and answers go only through answerRequest and alwaysAllow. An
// approval request also offers to always allow its tool. Its controls are
// disabled while an answer is sent and enabled again if the server refuses it.
// A typed answer is a draft, so it survives the form closing.
import {useId, useState} from 'react';
import {alwaysAllow, answerRequest} from '../api.ts';
import {draftOf, typed} from '../agent/drafts.ts';
import {submitsOnKey} from '../shared/submit-key.ts';
import {useStore} from '../store.ts';
import type {AttentionItem} from './types.ts';

interface AttentionFormProps {
  session: string;
  item: AttentionItem;
}

export function AttentionForm({session, item}: AttentionFormProps) {
  const [state, store] = useStore();
  const [sending, setSending] = useState(false);
  const id = useId();
  const key = `${session}/${item.id}`;
  const text = draftOf(state, key).text;

  const submit = async (answer: () => Promise<boolean>) => {
    if (sending) return;
    setSending(true);
    if (await answer()) store.update(s => typed(s, key, ''));
    else setSending(false);
  };
  const send = (choice: string) => submit(() => answerRequest(store, session, item.id, choice, text));
  const allowAlways = (all: boolean) => submit(() => alwaysAllow(store, session, item.id, all));
  const sendText = () => {
    if (text.trim()) send('');
  };

  return (
    <div className="attention-form space-y-3">
      {item.choices && item.choices.length > 0 && (
        <div className="attention-form-choices flex flex-wrap gap-2" role="group" aria-label="Choices">
          {item.choices.map((choice, i) => (
            <div className="attention-form-choice flex flex-col gap-1" key={choice}>
              <Button type="button" variant="secondary" data-choice={choice} disabled={sending} aria-describedby={item.notes?.[i] ? `${id}-note-${i}` : undefined} onClick={() => send(choice)}>
                {choice}
              </Button>
              {item.notes?.[i] && <span className="attention-form-note text-xs text-muted-foreground" id={`${id}-note-${i}`}>{item.notes[i]}</span>}
            </div>
          ))}
        </div>
      )}
      {item.grant && (
        <div className="attention-form-choices flex flex-wrap gap-2" role="group" aria-label="Always allow">
          <Button type="button" variant="secondary" data-always="session" disabled={sending} onClick={() => allowAlways(false)}>Always allow in this session</Button>
          <Button type="button" variant="secondary" data-always="all" disabled={sending} onClick={() => allowAlways(true)}>Always allow in all sessions</Button>
        </div>
      )}
      {item.free && (
        <div className="attention-form-reply space-y-2">
          <Label htmlFor={`${id}-reply`}>{item.choices?.length ? 'Other response' : 'Your response'}</Label>
          <div className="attention-form-entry flex items-center gap-2">
            <Input
              id={`${id}-reply`}
              aria-label={item.choices?.length ? 'Other response' : 'Your response'}
              value={text}
              disabled={sending}
              onChange={e => store.update(s => typed(s, key, e.currentTarget.value))}
              onKeyDown={e => e.target === e.currentTarget && submitsOnKey(e) && sendText()}
            />
            <Button type="button" variant="secondary" disabled={sending || !text.trim()} onClick={sendText}>Send</Button>
          </div>
        </div>
      )}
    </div>
  );
}
