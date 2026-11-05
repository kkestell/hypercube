// Confirmation is a question for the user; accept runs if they confirm with
// the primary button.
export interface Confirmation {
  message: string;
  detail: string;
  primary: string;
  accept: () => void;
}
