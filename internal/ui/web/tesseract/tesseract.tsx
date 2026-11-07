// A wireframe tesseract turning in 4D above a new session's composer.
// Lines fade with depth. It animates while mounted.
import {useEffect, useRef} from 'react';

const vertexShader = `#version 300 es
void main() {
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`;

// Draws thin, antialiased lines, dimmed by their depth.
const fragmentShader = `#version 300 es
precision highp float;
uniform vec2 resolution;
uniform vec4 segments[32];  // endpoints in pixels from the center
uniform float intensity[32];
uniform float width;        // line half-width in pixels
uniform float dark;
out vec4 color;

float sdSegment(vec2 p, vec2 a, vec2 b) {
  vec2 pa = p - a, ba = b - a;
  float h = clamp(dot(pa, ba) / dot(ba, ba), 0.0, 1.0);
  return length(pa - ba * h);
}

void main() {
  vec2 p = gl_FragCoord.xy - 0.5 * resolution;
  float brightness = 0.0;
  for (int i = 0; i < 32; i++) {
    float d = sdSegment(p, segments[i].xy, segments[i].zw);
    float line = 1.0 - smoothstep(width - 0.5, width + 0.5, d);
    brightness = max(brightness, intensity[i] * line);
  }
  color = vec4(vec3(brightness * dark), brightness);
}`;

// The 16 corners of [-1, 1]^4, and the 32 edges joining corners that differ in
// one coordinate.
const corners = Array.from({length: 16}, (_, i) => [0, 1, 2, 3].map(bit => (i >> bit) & 1 ? 1 : -1));
const edges: [number, number][] = [];
for (let i = 0; i < 16; i++) for (let bit = 0; bit < 4; bit++) if (!(i & (1 << bit))) edges.push([i, i | (1 << bit)]);

function rotate(p: number[], a: number, b: number, angle: number) {
  const c = Math.cos(angle), s = Math.sin(angle);
  [p[a], p[b]] = [p[a] * c - p[b] * s, p[a] * s + p[b] * c];
}

// project returns the pixel position from the center, and intensity, of each
// corner at time t seconds.
function project(t: number, width: number, height: number): [number, number, number][] {
  const size = Math.min(width, height) / 2;
  return corners.map((corner): [number, number, number] => {
    const p = corner.slice();
    rotate(p, 0, 3, t * 0.45); // XW
    rotate(p, 1, 3, t * 0.3); // YW
    const w = p[3];
    const k = 1 / (3 - w); // 4D perspective, eye at w = 3
    const q = [p[0] * k, p[1] * k, p[2] * k];
    // A fixed oblique view of the 3D shadow.
    rotate(q, 0, 2, 0.6);
    rotate(q, 1, 2, 0.45);
    const f = 1.4 / (2.5 - q[2]); // 3D perspective
    // Squared depth gives distant edges a stronger fade; w ranges over [-2, 2].
    return [q[0] * f * size, q[1] * f * size, ((w + 2) / 4) ** 2];
  });
}

function compile(gl: WebGL2RenderingContext, type: number, source: string): WebGLShader {
  const shader = gl.createShader(type);
  if (!shader) throw new Error('Tesseract shader: cannot create');
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) throw new Error(`Tesseract shader: ${gl.getShaderInfoLog(shader)}`);
  return shader;
}

// animate draws on the canvas until the returned function stops it.
function animate(canvas: HTMLCanvasElement): () => void {
  const gl = canvas.getContext('webgl2', {antialias: false, alpha: true});
  if (!gl) return () => {};

  const program = gl.createProgram();
  gl.attachShader(program, compile(gl, gl.VERTEX_SHADER, vertexShader));
  gl.attachShader(program, compile(gl, gl.FRAGMENT_SHADER, fragmentShader));
  gl.linkProgram(program);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) throw new Error(`Tesseract program: ${gl.getProgramInfoLog(program)}`);
  gl.useProgram(program);
  gl.bindVertexArray(gl.createVertexArray());
  const uniform = Object.fromEntries(['resolution', 'segments', 'intensity', 'width', 'dark']
    .map(name => [name, gl.getUniformLocation(program, name)]));

  const segments = new Float32Array(edges.length * 4);
  const intensity = new Float32Array(edges.length);
  let frame = 0;
  const draw = (ms: number) => {
    const width = Math.max(1, Math.round(canvas.clientWidth * devicePixelRatio));
    const height = Math.max(1, Math.round(canvas.clientHeight * devicePixelRatio));
    // Resizing clears the drawing buffer. Redraw in the same frame so a split
    // drag never presents the cleared buffer between animation frames.
    if (canvas.width !== width || canvas.height !== height) {
      canvas.width = width;
      canvas.height = height;
      gl.viewport(0, 0, width, height);
    }
    const projected = project(ms / 1000, canvas.width, canvas.height);
    edges.forEach(([i, j], n) => {
      const a = projected[i], b = projected[j];
      segments.set([a[0], a[1], b[0], b[1]], n * 4);
      intensity[n] = (a[2] + b[2]) / 2;
    });
    gl.uniform2f(uniform.resolution, canvas.width, canvas.height);
    gl.uniform4fv(uniform.segments, segments);
    gl.uniform1fv(uniform.intensity, intensity);
    gl.uniform1f(uniform.width, 0.5 * devicePixelRatio);
    gl.uniform1f(uniform.dark, document.documentElement.classList.contains('dark') ? 1 : 0);
    gl.drawArrays(gl.TRIANGLES, 0, 3);

    frame = requestAnimationFrame(draw);
  };
  frame = requestAnimationFrame(draw);
  return () => {
    cancelAnimationFrame(frame);
  };
}

export function Tesseract() {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => ref.current ? animate(ref.current) : undefined, []);
  return <canvas ref={ref} className="tesseract block min-h-0 w-full flex-1 pointer-events-none opacity-40" aria-hidden="true" />;
}
