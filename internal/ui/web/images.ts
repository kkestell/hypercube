// The images a prompt may carry: PNG, JPEG, GIF, or WebP, totalling at most
// 10 MiB, which the server checks again.

// Image is an image's MIME type and base64 data.
export interface Image {
  mime_type: string;
  data: string;
}

export const imageTypes = ['image/png', 'image/jpeg', 'image/gif', 'image/webp'];
export const maxImageBytes = 10 << 20;

// imageSrc returns the data URL of an image, or null unless it is one of the
// accepted types in base64.
export function imageSrc({mime_type: type, data}: Image): string | null {
  if (!imageTypes.includes(type) || !/^[A-Za-z0-9+/]+={0,2}$/.test(data)) return null;
  return `data:${type};base64,${data}`;
}

// imageBytes is the decoded size of base64 images.
export const imageBytes = (images: Image[]): number =>
  images.reduce((n, {data}) => n + data.length * 3 / 4 - (data.length - data.replace(/=+$/, '').length), 0);

// readImage reads a file as a prompt image.
export const readImage = (file: File) => new Promise<Image>((resolve, reject) => {
  const reader = new FileReader();
  reader.onload = () => {
    const url = String(reader.result);
    resolve({mime_type: file.type, data: url.slice(url.indexOf(',') + 1)});
  };
  reader.onerror = () => reject(reader.error);
  reader.readAsDataURL(file);
});
