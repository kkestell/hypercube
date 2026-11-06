// What the composer works with: profiles and drafts.
import type {Image} from '../images.ts';

// Profile is a configured harness, model, and effort a session can run with.
export interface Profile {
  name: string;
  description: string;
  harness: string;
  model: string;
  effort: string;
}

// Draft is what the user has typed and attached but not yet sent, and the
// number of attached images still being read.
export interface Draft {
  text: string;
  images: Image[];
  reading: number;
  choices?: string[];
}
