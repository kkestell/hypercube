// The images a prompt may carry: PNG, JPEG, GIF, or WebP, totalling at most
// 10 MiB, which the server checks again.

export const imageTypes = ['image/png', 'image/jpeg', 'image/gif', 'image/webp'];
export const maxImageBytes = 10 << 20;

// imageSrc returns the data URL of an image, or null unless it is one of the
// accepted types in base64.
export function imageSrc(image) {
  const {mime_type: type, data} = image ?? {};
  if (!imageTypes.includes(type)) return null;
  return typeof data === 'string' && /^[A-Za-z0-9+/]+={0,2}$/.test(data) ? `data:${type};base64,${data}` : null;
}

// imageBytes is the decoded size of base64 images.
export const imageBytes = images => images.reduce((n, image) => n + image.data.length * 3 / 4 - image.data.match(/=*$/)[0].length, 0);

// readImage reads a file as a prompt image.
export function readImage(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve({mime_type: file.type, data: reader.result.slice(reader.result.indexOf(',') + 1)});
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}
