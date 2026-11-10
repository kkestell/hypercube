import {Button} from '../components/ui/button';
import {Checkbox} from '../components/ui/checkbox';
import {Input} from '../components/ui/input';
import {Label} from '../components/ui/label';
import {RadioGroup, RadioGroupItem} from '../components/ui/radio-group';
// The answers to open attention requests: a radio button for each choice, or a
// checkbox when several may be picked, each with its note beneath, and an
// "Other" row with a text field when the request takes typed text.
// AttentionForm answers one request with a Submit button; the transcript and
// notification cards both draw it. An approval request also offers to always
// allow its tool. The transcript's pending card answers questions with
// QuestionsForm: one question at a time, with Back and Next and one Submit for
// all the questions asked together.
// Answers go only through answerRequest and alwaysAllow. Controls are disabled
// while answers are sent and enabled again if the server refuses one, and stay
// disabled in an ended session's read-only transcript. A choice and typed text
// are drafts, so they survive the form closing. A request that picks one
// choice takes either a choice or typed text, never both.
import {useId, useRef, useState, type ReactNode} from 'react';
import {alwaysAllow, answerRequest} from '../api.ts';
import {cleared, draftOf, selectedChoices, typed} from '../agent/drafts.ts';
import {submitsOnKey} from '../shared/submit-key.ts';
import type {State} from '../state.ts';
import {useStore} from '../store.ts';
import {Markdown} from './markdown.tsx';
import type {AttentionItem} from './types.ts';

// other marks the "Other" row among a draft's picked options and is its radio
// value; no choice's text equals it.
const other = '\0';

// answerOf is the drafted answer to a request: its draft key, the typed text,
// the picked options, the picked choices, and whether it can be sent.
function answerOf(state: State, session: string, item: AttentionItem) {
  const key = `${session}/${item.id}`;
  const {text, choices = []} = draftOf(state, key);
  const selected = (item.choices ?? []).filter(choice => choices.includes(choice));
  return {key, text, choices, selected, ready: selected.length > 0 || !!text.trim()};
}

// useSubmit runs a send and reports whether it is under way. A refused send
// enables the controls again.
function useSubmit() {
  const [sending, setSending] = useState(false);
  const submit = async (send: () => Promise<boolean>) => {
    if (sending) return;
    setSending(true);
    if (!await send()) setSending(false);
  };
  return [sending, submit] as const;
}

export function AttentionForm({session, item, ended}: {session: string; item: AttentionItem; ended?: boolean}) {
  const [state, store] = useStore();
  const [sending, submit] = useSubmit();
  const locked = sending || !!ended;
  const {key, text, selected, ready} = answerOf(state, session, item);

  const send = () => ready && submit(async () => {
    const sent = await answerRequest(store, session, item.id, selected, text);
    if (sent) store.update(s => cleared(s, key));
    return sent;
  });
  const allowAlways = (all: boolean) => submit(() => alwaysAllow(store, session, item.id, all));

  return (
    <div className="attention-form space-y-3">
      <AnswerFields session={session} item={item} locked={locked} onEnter={send} />
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

// QuestionsForm shows one of the questions at a time, with its progress when
// there are several. Next is enabled once the shown question has an answer,
// and on the last question Submit sends every answer in order. A question already answered leaves the form; a refused
// answer stops the rest, which stay with their drafts.
export function QuestionsForm({session, items, ended}: {session: string; items: AttentionItem[]; ended?: boolean}) {
  const [state, store] = useStore();
  const [sending, submit] = useSubmit();
  const [step, setStep] = useState(0);
  const locked = sending || !!ended;
  const shown = Math.min(step, items.length - 1);
  const item = items[shown];
  const answers = items.map(question => answerOf(state, session, question));
  const last = shown === items.length - 1;
  const ready = answers.every(answer => answer.ready);

  const send = () => ready && submit(async () => {
    for (const [i, question] of items.entries()) {
      const {key, selected, text} = answers[i];
      if (!await answerRequest(store, session, question.id, selected, text)) return false;
      store.update(s => cleared(s, key));
    }
    return true;
  });
  const next = () => answers[shown].ready && (last ? send() : setStep(shown + 1));

  return (
    <div className="questions-form space-y-3">
      <div className="attention-item-question"><Markdown text={item.text ?? ''} style="chat" /></div>
      <AnswerFields key={item.id} session={session} item={item} locked={locked} onEnter={next} />
      <div className="attention-form-actions flex items-center justify-end gap-2">
        {items.length > 1 && <>
          <span className="questions-form-progress mr-auto text-xs text-muted-foreground">Question {shown + 1} of {items.length}</span>
          <Button type="button" variant="secondary" size="sm" className="questions-form-back" disabled={locked || shown === 0} onClick={() => setStep(shown - 1)}>Back</Button>
        </>}
        {last
          ? <Button type="button" size="sm" className="attention-form-submit" disabled={locked || !ready} onClick={send}>Submit</Button>
          : <Button type="button" size="sm" className="questions-form-next" disabled={locked || !answers[shown].ready} onClick={next}>Next</Button>}
      </div>
    </div>
  );
}

// AnswerFields draws a request's choices and its "Other" field. Enter in the
// field calls onEnter.
function AnswerFields({session, item, locked, onEnter}: {session: string; item: AttentionItem; locked: boolean; onEnter: () => void}) {
  const [state, store] = useStore();
  const id = useId();
  const reply = useRef<HTMLInputElement>(null);
  const {key, text, choices, selected} = answerOf(state, session, item);
  const options = item.choices ?? [];
  const label = options.length ? 'Other response' : 'Your response';

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
      onKeyDown={e => e.target === e.currentTarget && submitsOnKey(e) && onEnter()} />
  );

  if (!options.length) return item.free ? field : null;
  return item.multi ? (
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
  );
}
