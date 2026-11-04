// An item's images that the page can show, as thumbnails no wider than the
// transcript.
import {imageSrc, type Image} from '../images.ts';

export function TranscriptImages({images = []}: {images?: Image[]}) {
  const sources = images.map(imageSrc).filter(src => src !== null);
  if (!sources.length) return null;
  return <div className="transcript-images flex flex-wrap gap-2">{sources.map((src, i) => <img key={i} className="transcript-images-image max-h-64 max-w-full object-contain" src={src} alt="Image" />)}</div>;
}
