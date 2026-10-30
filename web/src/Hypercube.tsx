import { useEffect, useRef } from "react";

const vertexShader = `#version 300 es
void main() {
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`;

// Draws the edges like an oscilloscope beam: each pixel sums a 1/distance glow from every edge.
const fragmentShader = `#version 300 es
precision highp float;
uniform vec2 resolution;
uniform vec4 segments[32];  // endpoints in pixels from the center
uniform float intensity[32];
uniform float core;         // pixels of full brightness around the beam
uniform float flicker;      // beam brightness this frame
uniform float time;
out vec4 color;
const vec3 background = vec3(20.0 / 255.0);
const vec3 phosphor = vec3(87.0, 85.0, 247.0) / 255.0;

float hash(vec2 p) { return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453); }

float sdSegment(vec2 p, vec2 a, vec2 b) {
  vec2 pa = p - a, ba = b - a;
  float h = clamp(dot(pa, ba) / dot(ba, ba), 0.0, 1.0);
  return length(pa - ba * h);
}

void main() {
  vec2 p = gl_FragCoord.xy - 0.5 * resolution;
  float hits = 0.0;
  for (int i = 0; i < 32; i++) hits += intensity[i] * min(1.0, core / sdSegment(p, segments[i].xy, segments[i].zw));
  vec3 c = mix(background, phosphor, hits * flicker);

  vec2 q = gl_FragCoord.xy / resolution - 0.5;
  c *= 1.0 - dot(q, q) * 2.0;
  // Grain, which also hides banding in the halo.
  c += (hash(gl_FragCoord.xy + fract(time) * 1000.0) - 0.5) * 0.05;
  color = vec4(c, 1.0);
}`;

// The 16 corners of [-1, 1]^4, and the 32 edges joining corners that differ in one coordinate.
const corners = Array.from({ length: 16 }, (_, i) => [0, 1, 2, 3].map(bit => ((i >> bit) & 1 ? 1 : -1)));
const edges: [number, number][] = [];
for (let i = 0; i < 16; i++) for (let bit = 0; bit < 4; bit++) if (!(i & (1 << bit))) edges.push([i, i | (1 << bit)]);

function rotate(p: number[], a: number, b: number, angle: number) {
  const c = Math.cos(angle);
  const s = Math.sin(angle);
  const [pa, pb] = [p[a]!, p[b]!];
  p[a] = pa * c - pb * s;
  p[b] = pa * s + pb * c;
}

/** Pixel position from the center, and intensity, for each corner at time `t` seconds. */
function project(t: number, width: number, height: number) {
  const size = Math.min(width, height) / 2;
  return corners.map(corner => {
    const p = corner.slice();
    rotate(p, 0, 3, t * 0.45); // XW
    rotate(p, 1, 3, t * 0.3); // YW
    const w = p[3]!;
    const k = 1 / (3 - w); // 4D perspective, eye at w = 3
    const q = [p[0]! * k, p[1]! * k, p[2]! * k];
    // A fixed oblique view of the 3D shadow.
    rotate(q, 0, 2, 0.6);
    rotate(q, 1, 2, 0.45);
    const f = 1.4 / (2.5 - q[2]!); // 3D perspective
    // Edges further along w are fainter; w ranges over [-2, 2].
    return [q[0]! * f * size, q[1]! * f * size, 0.25 + (0.75 * (w + 2)) / 4] as const;
  });
}

function compile(gl: WebGL2RenderingContext, type: number, source: string) {
  const shader = gl.createShader(type)!;
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) throw new Error(`Hypercube shader: ${gl.getShaderInfoLog(shader)}`);
  return shader;
}

/** A wireframe tesseract turning in 4D, drawn as a flickering oscilloscope trace to fill its parent. */
export function Hypercube() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current!;
    const gl = canvas.getContext("webgl2", { antialias: false, alpha: false });
    if (!gl) return;

    const program = gl.createProgram();
    gl.attachShader(program, compile(gl, gl.VERTEX_SHADER, vertexShader));
    gl.attachShader(program, compile(gl, gl.FRAGMENT_SHADER, fragmentShader));
    gl.linkProgram(program);
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) throw new Error(`Hypercube program: ${gl.getProgramInfoLog(program)}`);
    gl.useProgram(program);
    gl.bindVertexArray(gl.createVertexArray());
    const uniform = (name: string) => gl.getUniformLocation(program, name);
    const resolution = uniform("resolution");
    const segmentsUniform = uniform("segments");
    const intensityUniform = uniform("intensity");
    const core = uniform("core");
    const flickerUniform = uniform("flicker");
    const time = uniform("time");

    const resize = () => {
      canvas.width = Math.max(1, Math.round(canvas.clientWidth * devicePixelRatio));
      canvas.height = Math.max(1, Math.round(canvas.clientHeight * devicePixelRatio));
      gl.viewport(0, 0, canvas.width, canvas.height);
    };
    resize();
    const observer = new ResizeObserver(resize);
    observer.observe(canvas);

    const segments = new Float32Array(edges.length * 4);
    const intensity = new Float32Array(edges.length);
    // Lightly smoothed noise, so the beam wavers in brightness rather than strobing.
    let flicker = 0;
    let frame = requestAnimationFrame(function draw(ms) {
      const projected = project(ms / 1000, canvas.width, canvas.height);
      edges.forEach(([i, j], n) => {
        const [a, b] = [projected[i]!, projected[j]!];
        segments.set([a[0], a[1], b[0], b[1]], n * 4);
        intensity[n] = (a[2] + b[2]) / 2;
      });
      flicker += (Math.random() - flicker) * 0.7;

      gl.uniform2f(resolution, canvas.width, canvas.height);
      gl.uniform4fv(segmentsUniform, segments);
      gl.uniform1fv(intensityUniform, intensity);
      gl.uniform1f(core, 0.8 * devicePixelRatio);
      gl.uniform1f(flickerUniform, 1 - 0.1 * flicker);
      gl.uniform1f(time, ms / 1000);
      gl.drawArrays(gl.TRIANGLES, 0, 3);

      // Fades in once the first frame is drawn.
      canvas.style.opacity = "1";
      frame = requestAnimationFrame(draw);
    });

    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
    };
  }, []);

  return <canvas ref={canvasRef} className="pointer-events-none absolute inset-0 h-full w-full opacity-0 transition-opacity duration-1000" />;
}
