// An item's images that the page can show, as thumbnails no wider than the
// transcript.
import {imageSrc} from '../images.js';
import './transcript-images.css';

export function TranscriptImages({images}) {
  const sources = (images ?? []).map(imageSrc).filter(Boolean);
  if (!sources.length) return null;
  return <div class="transcript-images">{sources.map(src => <img class="transcript-images-image" src={src} alt="Image" />)}</div>;
}
