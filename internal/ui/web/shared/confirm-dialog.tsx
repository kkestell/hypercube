// Confirmation state and acceptance stay in the store; Radix owns modal focus.
// The last question stays drawn while the dialog closes.
import {useState} from 'react';
import {useStore} from '../store.ts';
import {AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle} from '../components/ui/alert-dialog';

export function ConfirmDialog() {
  const [state, store] = useStore();
  const {confirmation} = state;
  const [shown, setShown] = useState(confirmation);
  if (confirmation && confirmation !== shown) setShown(confirmation);
  const close = () => store.update(s => ({...s, confirmation: null}));
  return <AlertDialog open={!!confirmation} onOpenChange={open => { if (!open) close(); }}>
    {shown && <AlertDialogContent>
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
