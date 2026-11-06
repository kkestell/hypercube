import {Button} from '../components/ui/button';
import {Checkbox} from '../components/ui/checkbox';
import {Input} from '../components/ui/input';
import {Label} from '../components/ui/label';
import {RadioGroup, RadioGroupItem} from '../components/ui/radio-group';
// The answers to an open attention request: a radio button for each choice,
// or a checkbox when several may be picked, each with its note beneath, an
// "Other" row with a text field when the request takes typed text, and a
// Submit button. The transcript and notification cards both draw it, and
// answers go only through answerRequest and alwaysAllow. An approval request
// also offers to always allow its tool. Its controls are disabled while an
// answer is sent and enabled again if the server refuses it, and stay disabled
// in an ended session's read-only transcript. A choice and typed text are
// drafts, so they survive the form closing. A request that picks one choice
// takes either a choice or typed text, never both.
import {useId, useRef, useState, type ReactNode} from 'react';
import {alwaysAllow, answerRequest} from '../api.ts';
import {cleared, draftOf, selectedChoices, typed} from '../agent/drafts.ts';
import {submitsOnKey} from '../shared/submit-key.ts';
import {useStore} from '../store.ts';
import type {AttentionItem} from './types.ts';

// other marks the "Other" row among a draft's picked options and is its radio
// value; no choice's text equals it.
const other = '\0';

interface AttentionFormProps {
  session: string;
  item: AttentionItem;
  ended?: boolean;
}

export function AttentionForm({session, item, ended}: AttentionFormProps) {
  const [state, store] = useStore();
  const [sending, setSending] = useState(false);
  const locked = sending || !!ended;
  const id = useId();
  const reply = useRef<HTMLInputElement>(null);
  const key = `${session}/${item.id}`;
  const {text, choices = []} = draftOf(state, key);
  const options = item.choices ?? [];
  const selected = options.filter(choice => choices.includes(choice));
  const ready = selected.length > 0 || !!text.trim();
  const label = options.length ? 'Other response' : 'Your response';

  const submit = async (answer: () => Promise<boolean>) => {
    if (sending) return;
    setSending(true);
    if (await answer()) store.update(s => cleared(s, key));
    else setSending(false);
  };
  const send = () => ready && submit(() => answerRequest(store, session, item.id, selected, text));
  const allowAlways = (all: boolean) => submit(() => alwaysAllow(store, session, item.id, all));
  const pick = (picked: string[]) => store.update(s => selectedChoices(item.multi ? s : typed(s, key, ''), key, picked));
  // Typing picks Other, and in a request that picks one choice drops the choice.
  const type = (value: string) => store.update(s => {
    const typedText = typed(s, key, value);
    if (!options.length || choices.includes(other)) return typedText;
    return selectedChoices(typedText, key, item.multi ? [...choices, other] : [other]);
  });
  const clearOther = () => store.update(s => typed(selectedChoices(s, key, choices.filter(value => value !== other)), key, ''));

  const note = (i: number) => item.notes?.[i] && <span className="attention-form-note col-start-2 text-xs text-muted-foreground" id={`${id}-note-${i}`}>{item.notes[i]}</span>;
  const row = (slot: string, control: ReactNode, content: ReactNode, extra?: ReactNode) => <div key={slot} className="attention-form-choice grid grid-cols-[auto_1fr] items-start gap-x-3 gap-y-0.5 py-1.5">{control}{content}{extra}</div>;
  const choiceLabel = (i: number, choice: string) => <Label htmlFor={`${id}-choice-${i}`} className="min-w-0 whitespace-normal break-words font-normal leading-5">{choice}</Label>;
  const field = (
    <Input ref={reply} id={`${id}-reply`} className="h-8" aria-label={label} placeholder={options.length ? 'Other' : label} value={text} disabled={locked}
      onChange={e => type(e.currentTarget.value)}
      onKeyDown={e => e.target === e.currentTarget && submitsOnKey(e) && send()} />
  );

  return (
    <div className="attention-form space-y-3">
      {options.length > 0 && (item.multi ? (
        <div className="attention-form-choices" role="group" aria-label="Choices">
          {options.map((choice, i) => row(choice,
            <Checkbox id={`${id}-choice-${i}`} className="mt-0.5" checked={selected.includes(choice)} disabled={locked} aria-describedby={item.notes?.[i] ? `${id}-note-${i}` : undefined}
              onCheckedChange={checked => pick(checked ? [...choices, choice] : choices.filter(value => value !== choice))} />,
            choiceLabel(i, choice), note(i)))}
          {item.free && row(other,
            <Checkbox id={`${id}-other`} className="mt-1.5" checked={choices.includes(other)} disabled={locked} aria-label="Other"
              onCheckedChange={checked => { if (checked) { pick([...choices, other]); reply.current?.focus(); } else clearOther(); }} />, field)}
        </div>
      ) : (
        <RadioGroup className="attention-form-choices gap-0" aria-label="Choices" value={selected[0] ?? (choices.includes(other) ? other : '')} disabled={locked}
          onValueChange={value => { pick([value]); if (value === other) reply.current?.focus(); }}>
          {options.map((choice, i) => row(choice,
            <RadioGroupItem id={`${id}-choice-${i}`} className="mt-0.5" value={choice} data-choice={choice} aria-describedby={item.notes?.[i] ? `${id}-note-${i}` : undefined} />,
            choiceLabel(i, choice), note(i)))}
          {item.free && row(other, <RadioGroupItem id={`${id}-other`} className="mt-1.5" value={other} aria-label="Other" />, field)}
        </RadioGroup>
      ))}
      {options.length === 0 && item.free && field}
      <div className="attention-form-actions flex flex-wrap items-center justify-end gap-2">
        {item.grant && (
          <div className="mr-auto flex flex-wrap gap-2" role="group" aria-label="Always allow">
            <Button type="button" variant="secondary" size="sm" data-always="session" disabled={locked} onClick={() => allowAlways(false)}>Always allow in this session</Button>
            <Button type="button" variant="secondary" size="sm" data-always="all" disabled={locked} onClick={() => allowAlways(true)}>Always allow in all sessions</Button>
          </div>
        )}
        <Button type="button" size="sm" className="attention-form-submit" disabled={locked || !ready} onClick={send}>Submit</Button>
      </div>
    </div>
  );
}
