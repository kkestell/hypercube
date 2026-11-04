// Confirmation state and acceptance stay in the store; Radix owns modal focus.
import {useStore} from '../store.ts';
import {AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle} from '../components/ui/alert-dialog';

export function ConfirmDialog() {
  const [state, store] = useStore();
  const {confirmation} = state;
  const close = () => store.update(s => ({...s, confirmation: null}));
  return <AlertDialog open={!!confirmation} onOpenChange={open => { if (!open) close(); }}>
    {confirmation && <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{confirmation.message}</AlertDialogTitle>
        <AlertDialogDescription>{confirmation.detail}</AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>Cancel</AlertDialogCancel>
        <AlertDialogAction onClick={() => { close(); confirmation.accept(); }}>{confirmation.primary}</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>}
  </AlertDialog>;
}
