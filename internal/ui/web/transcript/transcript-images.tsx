// An item's images as thumbnails no wider than the transcript. Each image is
// the address the server links it at, or a data URL for a prompt still being
// sent.
export function TranscriptImages({images = []}: {images?: string[]}) {
  if (!images.length) return null;
  return <div className="transcript-images flex flex-wrap gap-2">{images.map((src, i) => <img key={i} className="transcript-images-image max-h-64 max-w-full object-contain" src={src} alt="Image" />)}</div>;
}
