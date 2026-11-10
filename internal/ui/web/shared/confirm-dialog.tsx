// Confirmation state and acceptance stay in the store. The last question
// stays drawn while the dialog closes, and closing returns the focus to where
// it was when the question was asked.
import {useState} from 'react';
import {useStore} from '../store.ts';
import {useDialogFocus} from './focus.ts';
import {AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle} from '../components/ui/alert-dialog';

export function ConfirmDialog() {
  const [state, store] = useStore();
  const {confirmation} = state;
  const [shown, setShown] = useState(confirmation);
  if (confirmation && confirmation !== shown) setShown(confirmation);
  const focus = useDialogFocus(!!confirmation);
  const close = () => store.update(s => ({...s, confirmation: null}));
  return <AlertDialog open={!!confirmation} onOpenChange={open => { if (!open) close(); }}>
    {shown && <AlertDialogContent {...focus}>
      <AlertDialogHeader>
        <AlertDialogTitle>{shown.message}</AlertDialogTitle>
        <AlertDialogDescription>{shown.detail}</AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>Cancel</AlertDialogCancel>
        <AlertDialogAction onClick={() => { close(); shown.accept(); }}>{shown.primary}</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>}
  </AlertDialog>;
}
