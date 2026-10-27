# Models

This document picks a small, medium, and large model and effort level for
coding agents from OpenAI, from Anthropic, and from the other labs, based on
Artificial Analysis benchmark data collected on 2026-10-26.

| Tier   | OpenAI                | Anthropic               | Other labs                    |
| ------ | --------------------- | ----------------------- | ----------------------------- |
| Small  | GPT-6.1 Terra, medium | Claude Haiku 5.5, xhigh | MiMo-V2.6-Flash, default      |
| Medium | GPT-6.1 Sol, high     | Claude Sonnet 5.5, high | DeepSeek V4.1 Flash 1017, max |
| Large  | GPT-6.1 Sol, xhigh    | Claude Opus 5.5, medium | Kimi K3.1 Pro, max            |

The OpenAI candidates were GPT-6.1 Sol, GPT-6.1 Terra, and GPT-6 Luna, and the
Anthropic candidates were Claude Opus 5.5, Sonnet 5.5, and Haiku 5.5, each at
every effort level it supports. The other labs are covered in
[Other labs](#other-labs).

## Method

A setting is on the Pareto frontier when no other setting scores at least as
high for less cost. The recommendations come from three measures:

- **Terminal-Bench 4.0**: agentic coding and terminal use. Artificial Analysis
  reports score, cost per task, time per task, and output tokens per task for
  every effort level.
- **FrontierCode 1.1**: agentic software engineering. Artificial Analysis
  reports a score for every effort level, but no per-level cost or time.
- **Artificial Analysis Intelligence Index**: a general mix of ten evaluations,
  including Terminal-Bench 4.0. It is the broadest measure but not specific to
  coding.

Each effort level has its own model page, for example
`https://artificialanalysis.ai/models/gpt-6-1-sol-high`. The page embeds the
level's full benchmark record, including the per-evaluation cost, time, and
token counts. The figures below were read from those records.

Differences of one or two points are within run-to-run noise. Costs are at list
API prices. Times are wall-clock time per task through the vendor APIs.

## Recommendations

### OpenAI

- **Small: Terra medium.** Terminal-Bench 32.3% at $0.28 and 3.5 minutes per
  task; FrontierCode 43.7%. Terra low costs $0.11 but falls to 15.2% on
  Terminal-Bench. Luna scores 12.6% on Terminal-Bench even at max.
- **Medium: Sol high.** Terminal-Bench 51.5% at $0.83 and 12.7 minutes;
  FrontierCode 46.6%. Sol medium is the cheaper alternative at 48.0% and 43.3%
  for $0.61.
- **Large: Sol xhigh.** Terminal-Bench 54.0% at $1.03 and 15 minutes;
  FrontierCode 46.3%. Sol max adds two to three points on each benchmark for
  77% more cost and 70% more time.

### Anthropic

- **Small: Haiku xhigh.** Terminal-Bench 29.3% at $0.46 and 12.5 minutes;
  FrontierCode 45.5%. Haiku high is faster at 9.5 minutes but scores 21.7% on
  Terminal-Bench. Sonnet medium is faster still but scores 29.3% on
  FrontierCode and costs 3.4 times as much.
- **Medium: Sonnet high.** Terminal-Bench 43.9% at $2.19 and 7.4 minutes;
  FrontierCode 44.3%. Sonnet xhigh costs more than Opus medium and scores lower
  on both benchmarks combined.
- **Large: Opus medium.** Terminal-Bench 52.5% at $4.04 and 9.7 minutes;
  FrontierCode 57.1%, the highest of any setting. Opus high trades 4.1 points
  more on Terminal-Bench for 3.1 points less on FrontierCode at 27% more cost.
  Opus xhigh adds three Terminal-Bench points over high for 71% more cost. Opus max
  scores the same as xhigh on Terminal-Bench and costs more.

### Across vendors

At every tier the OpenAI pick costs less per task. Terra medium beats Haiku
xhigh on Terminal-Bench at 60% of the cost and under a third of the time. Sol
high beats Sonnet high on both benchmarks at 38% of the cost. Opus medium leads
FrontierCode by 11 points over Sol xhigh, at about four times the cost.

## Terminal-Bench 4.0

Each cell is score, cost per task, and minutes per task. ✗ marks a setting off
its own model's frontier.

| Model  | none                 | low                 | medium                | high                 | xhigh                | max                     |
| ------ | -------------------- | ------------------- | --------------------- | -------------------- | -------------------- | ----------------------- |
| Luna   | 1.5% / $0.04 / 1.9   | 0.0% / $0.01 / 0.5  | 2.5% / $0.07 / —      | 4.5% / $0.12 / 10.3  | 8.1% / $0.20 / 14.1  | 12.6% / $0.24 / 18.1    |
| Terra  | 26.8% / $0.38 / 3.0 ✗ | 15.2% / $0.11 / 1.4 | 32.3% / $0.28 / 3.5   | 41.9% / $0.44 / 5.4  | 46.5% / $0.66 / 8.7  | 55.6% / $1.49 / 18.5    |
| Sol    | —                    | 30.8% / $0.38 / 5.8 | 48.0% / $0.61 / 10.1  | 51.5% / $0.83 / 12.7 | 54.0% / $1.03 / 15.0 | 56.1% / $1.82 / 25.7    |
| Haiku  | —                    | 12.6% / $0.12 / 4.1 | 15.2% / $0.19 / 6.4   | 21.7% / $0.33 / 9.5  | 29.3% / $0.46 / 12.5 | 32.8% / $0.65 / 15.2    |
| Sonnet | —                    | 20.7% / $1.28 / 3.9 | 29.8% / $1.58 / 5.0   | 43.9% / $2.19 / 7.4  | 57.1% / $6.08 / 18.3 | 63.6% / $12.65 / 28.7   |
| Opus   | —                    | 31.3% / $2.08 / 5.0 | 52.5% / $4.04 / 9.7   | 56.6% / $5.12 / 13.1 | 59.6% / $8.78 / 21.3 | 59.6% / $13.11 / 27.4 ✗ |

Output tokens per task:

| Model  | none | low | medium | high | xhigh | max  |
| ------ | ---- | --- | ------ | ---- | ----- | ---- |
| Luna   | 14k  | 4k  | 45k    | 74k  | 104k  | 141k |
| Terra  | 17k  | 7k  | 19k    | 30k  | 47k   | 101k |
| Sol    | —    | 17k | 32k    | 44k  | 57k   | 108k |
| Haiku  | —    | 70k | 107k   | 166k | 232k  | 347k |
| Sonnet | —    | 40k | 52k    | 77k  | 192k  | 382k |
| Opus   | —    | 38k | 76k    | 100k | 169k  | 253k |

Frontiers:

- **OpenAI:** Terra low, medium, and high; Sol medium, high, and xhigh; Terra
  max; Sol max. Luna's settings sit below 13%.
- **Anthropic:** every Haiku setting; Sonnet high; Opus medium and high; Sonnet
  xhigh; Opus xhigh; Sonnet max.
- **All six models:** Terra low, medium, and high; Sol medium, high, and xhigh;
  Terra max; Sol max; Opus high; Sonnet xhigh; Opus xhigh; Sonnet max. Sonnet
  max has the top score, 63.6%.

## FrontierCode 1.1

Score only. Scores often fall as effort rises, so small differences are noise.

| Model  | none  | low   | medium | high  | xhigh | max   |
| ------ | ----- | ----- | ------ | ----- | ----- | ----- |
| Luna   | 13.8% | 14.1% | 31.8%  | 32.0% | 33.2% | 41.9% |
| Terra  | 41.0% | 36.3% | 43.7%  | 37.2% | 41.4% | 36.0% |
| Sol    | —     | 36.7% | 43.3%  | 46.6% | 46.3% | 49.1% |
| Haiku  | —     | 29.3% | 40.4%  | 43.6% | 45.5% | 41.8% |
| Sonnet | —     | 26.4% | 29.3%  | 44.3% | 46.1% | 45.8% |
| Opus   | —     | 42.7% | 57.1%  | 54.0% | 53.5% | 52.2% |

Anthropic attributes Sonnet 5.5's lower FrontierCode score at max than at
xhigh to more frequent code-review subagent runs that led to timeouts or
out-of-scope edits.

## Intelligence Index

Each cell is index score and cost per index task. ✗ marks a setting off the
frontier among its vendor's models.

| Model  | none          | low             | medium        | high          | xhigh         | max             |
| ------ | ------------- | --------------- | ------------- | ------------- | ------------- | --------------- |
| Luna   | 18 / $0.01 ✗  | 22 / $0.0045    | 30 / $0.02    | 33 / $0.03    | 35 / $0.04    | 38 / $0.07      |
| Terra  | 33 / $0.11 ✗  | 36 / $0.05      | 41 / $0.10    | 44 / $0.13    | 45 / $0.19    | 48 / $0.40 ✗    |
| Sol    | —             | 42 / $0.13 ✗    | 48 / $0.21    | 50 / $0.32    | 51 / $0.39    | 52 / $0.72      |
| Haiku  | —             | 29 / $0.02      | 34 / $0.05    | 38 / $0.08    | 41 / $0.12    | 43 / $0.21      |
| Sonnet | —             | 36 / $0.35 ✗    | 41 / $0.48 ✗  | 47 / $0.88    | 52 / $2.01 ✗  | 56 / $5.46 ✗    |
| Opus   | —             | 42 / $0.55 ✗    | 51 / $1.34    | 54 / $1.82    | 56 / $3.46    | 58 / $5.98      |

Across all six models, the frontier is Luna low through max, Terra low through
xhigh, Sol medium through max, and Opus high, xhigh, and max. No Haiku or
Sonnet setting is on it.

## Coding Agent Index

The Coding Agent Index averages FrontierCode, Terminal-Bench 4.0, and
SWE-Atlas-QnA, run inside a production coding agent. Artificial Analysis
reports Terra at every effort level and the other models at max only. GPT
models ran in Codex and Claude models in Claude Code, so each score measures
the model and its harness together.

| Agent and model     | Index | Cost per task | Time per task | Tokens per task |
| ------------------- | ----- | ------------- | ------------- | --------------- |
| Codex, Luna max     | 31    | $0.16         | 21.3m         | 8.9M            |
| Codex, Terra low    | 35    | $0.20         | 6.3m          | 1.9M            |
| Codex, Terra none   | 39    | $0.45         | 9.5m          | 5.8M            |
| Codex, Terra medium | 42    | $0.39         | 10.2m         | 4.1M            |
| Codex, Terra high   | 46    | $0.54         | 12.7m         | 5.6M            |
| Codex, Terra xhigh  | 48    | $0.69         | 16.4m         | 6.9M            |
| Codex, Terra max    | 48    | $1.24         | 26.1m         | 12.3M           |
| Codex, Sol max      | 54    | $1.50         | 23.7m         | 3.9M            |
| Claude Code, Haiku max  | 37 | $2.67       | 1.2h          | 16.1M           |
| Claude Code, Sonnet max | 59 | $13.30      | 1.3h          | 24.1M           |
| Claude Code, Opus max   | 62 | $11.42      | 1.0h          | 14.5M           |

Artificial Analysis places Terra low and xhigh on the Coding Agent Index cost
frontier.

## Other labs

The candidates were the current models from DeepSeek, Moonshot (Kimi), Z AI
(GLM), Xiaomi (MiMo), Meta (Muse), Alibaba (Qwen), MiniMax, StepFun, Mistral,
NVIDIA (Nemotron), InclusionAI (Ling), Meituan (LongCat), Thinking Machines
(Inkling), Reflection (Beam), MBZUAI (K2 Horizon), and OpenAI's open-weight
gpt-oss-120b. Artificial Analysis measures most of them at one effort level,
usually max.

### Recommendations

- **Small: MiMo-V2.6-Flash.** Terminal-Bench 22.7% at $0.23 per task;
  FrontierCode 42.1%; Intelligence Index 38 at $0.06 per task, the cheapest
  other-lab model scoring above 35. It is slow: 69.7 minutes per Terminal-Bench task.
  OpenRouter offers no effort levels for it.
- **Medium: DeepSeek V4.1 Flash 1017, max.** Terminal-Bench 33.8% at $0.88 and
  15.9 minutes; FrontierCode 43.7%; Intelligence Index 45 at $0.26. In
  DeepSeek's own harness it scores 46 on the Coding Agent Index at $0.30 per
  task. OpenRouter's default effort for this model is high, so set max
  explicitly. MiMo-V2.6-Pro scores slightly higher (34.8% and 45.4%) for half
  the cost, $0.44, but takes 51.3 minutes per Terminal-Bench task.
- **Large: Kimi K3.1 Pro, max.** Terminal-Bench 67.2%, the highest of any model
  measured, including Opus and Sonnet, at $5.27 and 35.7 minutes; FrontierCode
  47.1%; Intelligence Index 55 at $2.11. Its weights are not open, and it was
  released on 2026-10-23, so it has no Coding Agent Index result yet. The
  strongest open-weight model on Terminal-Bench is GLM-5.3 at max: 41.9% at
  $8.06, and 49 on the Coding Agent Index in Opencode at $5.23.

### Effort levels

Where Artificial Analysis measured more than one level, the highest level was
best:

- **DeepSeek V4.1 Flash:** non-reasoning scores 5.6% on Terminal-Bench at
  $1.79; max scores 26.8% at $1.46.
- **GLM-5.3:** low scores 34.8% at $6.40 and 34.9% on FrontierCode; max scores
  41.9% at $8.06 and 40.1%.
- **Kimi K3:** low and max both score 12.6%; low costs $9.47 and max $5.09.
- **Muse Spark 1.3:** xhigh scores 16.7% at $5.03; max scores 33.3% at $5.58.
- **Qwen3.8 27B:** no level scores above 5.6%.

### Compared with OpenAI and Anthropic

GPT-6.1 Terra and Sol score higher on Terminal-Bench than nearly every
other-lab model at the same cost, and finish tasks several times faster. Terra
medium scores 32.3% at $0.28 and 3.5 minutes, against DeepSeek V4.1 Flash
1017's 33.8% at $0.88 and 15.9 minutes. Across all models, the only other-lab
models on the Terminal-Bench frontier are MiMo-V2.6-Flash and Kimi K3.1 Pro.
On the Intelligence Index they are MiMo-V2.6-Flash, MiMo-V2.6-Pro, and Kimi
K3.1 Pro.

### Data

Terminal-Bench 4.0 cells are score, cost per task, and minutes per task. The
Intelligence Index cells are score and cost per task. ✓ marks open weights.

| Model                         | Open | Terminal-Bench 4.0     | FrontierCode | Intelligence Index |
| ----------------------------- | ---- | ---------------------- | ------------ | ------------------ |
| Kimi K3.1 Pro                 |      | 67.2% / $5.27 / 35.7   | 47.1%        | 55 / $2.11         |
| Muse Wonder Full, max         |      | 57.1% / — / —          | 44.9%        | 56 / —             |
| Muse Wonder, max              |      | 52.0% / $12.21 / 14.2  | 46.5%        | 52 / $3.27         |
| GLM-5.3, max                  | ✓    | 41.9% / $8.06 / 27.5   | 40.1%        | 45 / $2.01         |
| Qwen3.8 Max                   |      | 38.9% / $18.68 / 89.1  | 43.2%        | 45 / $5.41         |
| GLM-5.3, low                  | ✓    | 34.8% / $6.40 / 23.2   | 34.9%        | 34 / $0.85         |
| MiMo-V2.6-Pro                 | ✓    | 34.8% / $0.44 / 51.3   | 45.4%        | 46 / $0.13         |
| DeepSeek V4.1 Flash 1017, max | ✓    | 33.8% / $0.88 / 15.9   | 43.7%        | 45 / $0.26         |
| Muse Spark 1.3, max           |      | 33.3% / $5.58 / 21.7   | 40.6%        | 48 / $1.31         |
| Step 5 Preview                | ✓    | 33.3% / $5.18 / 18.5   | 38.4%        | 44 / $1.03         |
| Ling 3.1 Flash                | ✓    | 33.3% / $5.49 / 22.1   | —            | 41 / $1.01         |
| GLM-5.3-Flash                 | ✓    | 32.8% / $0.89 / 31.4   | 41.0%        | 42 / $0.28         |
| DeepSeek V4.1 Flash, max      | ✓    | 26.8% / $1.46 / 19.9   | 44.0%        | 40 / $0.32         |
| Mistral Large 4 Preview       |      | 26.8% / $8.66 / 31.7   | 41.5%        | 38 / $1.61         |
| MiniMax-M3.1-Flash            |      | 25.8% / $0.92 / 29.1   | 38.6%        | 43 / $0.22         |
| Qwen3.8-Flash-Next            | ✓    | 25.3% / $0.88 / 56.7   | —            | 40 / $0.37         |
| MiMo-V2.6-Flash               | ✓    | 22.7% / $0.23 / 69.7   | 42.1%        | 38 / $0.06         |

The rest score below 20% on Terminal-Bench: Muse Spark 1.3 at xhigh (16.7%),
DeepSeek V4 Pro 0813 (14.1%), Kimi K3 (12.6%), Qwen3.8 2.4T A95B (11.1%),
Muse Spark Open (8.1%), Qwen3.8 27B (5.6% at best), and Beam 1.0, MiniMax-M3,
K2 Horizon 375B, Kimi K2.7 Code, Inkling, Muse Glimmer, Nemotron 3.5 Super VL
and Lightning, LongCat 2.0, Mistral Medium 3.5, Mistral Small 4, and
gpt-oss-120b (3% or less).

### Coding Agent Index

| Agent and model                            | Index | Cost per task | Time per task | Tokens per task |
| ------------------------------------------ | ----- | ------------- | ------------- | --------------- |
| Muse Code, Muse Wonder Full max            | 59    | —             | 34.9m         | 14.5M           |
| Muse Code, Muse Wonder max                 | 52    | $8.05         | 24.5m         | 14.5M           |
| Opencode, GLM-5.3                          | 49    | $5.23         | 56.2m         | 15.9M           |
| Codex, DeepSeek V4.1 Flash                 | 47    | $0.39         | 38.1m         | 27.2M           |
| DeepSeek Harness, DeepSeek V4.1 Flash 1017 | 46    | $0.30         | 21.4m         | 16.7M           |
| Muse Code, Muse Spark 1.3 max              | 46    | $3.91         | 23.3m         | 17.8M           |
| Claude Code, Step 5 Preview                | 43    | $1.84         | 1.0h          | 22.1M           |
| Kimi Code CLI, Kimi K3                     | 41    | $5.17         | 1.0h          | 9M              |
| Claude Code, Qwen3.8 Max                   | 39    | $3.80         | 1.3h          | 9.2M            |
| Muse Code, Muse Spark Open xhigh           | 26    | $3.54         | 26.0m         | 13.3M           |
| Mirror, Beam 1.0                           | 20    | —             | 38.7m         | 4.9M            |

## Prices

Prices are in dollars per million tokens. The list price is the model maker's
own API price, which Artificial Analysis uses for every cost figure in this
document.

### OpenAI and Anthropic

| Model             | Input | Cached input | Output |
| ----------------- | ----- | ------------ | ------ |
| GPT-6 Luna        | 0.10  | 0.01         | 0.50   |
| GPT-6.1 Terra     | 1.00  | 0.05         | 5.00   |
| GPT-6.1 Sol       | 2.00  | 0.10         | 10.00  |
| Claude Haiku 5.5  | 0.10  | 0.01         | 0.50   |
| Claude Sonnet 5.5 | 2.00  | 0.10         | 10.00  |
| Claude Opus 5.5   | 4.00  | 0.20         | 20.00  |

OpenAI also sells each GPT model at half price on its flex tier and at twice
the price on its fast tier, and Sol at six times the price on its ultrafast
tier. Anthropic sells Opus at twice the price in fast mode. Batch requests are
half price for both. Azure, Amazon Bedrock, and Google Vertex charge the list
price on global endpoints and 10% more on regional ones.

### Other labs

The last two columns are the cheapest and most expensive OpenRouter providers'
input and output prices, and the number of providers. Cheaper providers often
serve quantized weights. ✓ marks open weights.

| Model                       | Open | Input | Cached input | Output | OpenRouter input | OpenRouter output | Providers |
| --------------------------- | ---- | ----- | ------------ | ------ | ---------------- | ----------------- | --------- |
| Kimi K3.1 Pro               |      | 2.00  | 0.20         | 10.00  | 2.00–4.00        | 10.00–20.00       | 1         |
| Muse Wonder                 |      | 4.00  | 0.20         | 20.00  | 4.00             | 20.00             | 1         |
| GLM-5.3                     | ✓    | 1.40  | 0.26         | 4.40   | 0.07–2.80        | 1.40–8.80         | 37        |
| Qwen3.8 Max                 |      | 2.00  | 0.25         | 6.00   | 2.00             | 6.00              | 1         |
| MiMo-V2.6-Pro               | ✓    | 0.435 | 0.0036       | 0.87   | 0.39–0.52        | 0.78–1.04         | 5         |
| DeepSeek V4.1 Flash 1017    | ✓    | 0.30  | 0.006        | 1.20   | 0.03–0.33        | 0.01–1.32         | 30        |
| Muse Spark 1.3              |      | 1.25  | 0.15         | 4.25   | 1.25             | 4.25              | 1         |
| Muse Spark 1.3 Contributor  |      | 0.10  | 0.002        | 0.20   | 0.10             | 0.20              | 1         |
| Step 5 Preview              | ✓    | 1.00  | 0.10         | 2.70   | 1.00             | 2.70              | 4         |
| Ling 3.1 Flash              | ✓    | 0.30  | 0.06         | 0.90   | 0.075            | 0.225             | 2         |
| GLM-5.3-Flash               | ✓    | 0.15  | 0.03         | 0.50   | 0.04–0.28        | 0.01–1.20         | 34        |
| DeepSeek V4.1 Flash         | ✓    | 0.30  | 0.006        | 1.20   | 0.03–0.60        | 0.01–2.40         | 33        |
| Mistral Large 4 Preview     |      | 1.36  | 0.14         | 4.18   | 1.36–1.50        | 4.18–4.60         | 1         |
| MiniMax-M3.1-Flash          |      | 0.15  | 0.015        | 0.60   | 0.14–0.15        | 0.57–0.60         | 10        |
| Qwen3.8-Flash-Next          | ✓    | 0.15  | 0.016        | 0.47   | —                | —                 | 0         |
| MiMo-V2.6-Flash             | ✓    | 0.14  | 0.0028       | 0.28   | 0.09–0.18        | 0.18–0.35         | 10        |
| Muse Spark Open             | ✓    | 1.25  | 0.15         | 4.25   | 1.25             | 4.25              | 6         |
| DeepSeek V4 Pro 0813        | ✓    | 1.32  | 0.044        | 3.96   | 0.22–1.65        | 0.80–8.00         | 23        |
| Kimi K3                     | ✓    | 3.00  | 0.30         | 15.00  | 0.15–4.50        | 8.25–28.13        | 21        |
| Qwen3.8 2.4T A95B           | ✓    | 2.00  | 0.25         | 6.00   | 2.00             | 6.00              | 5         |
| Qwen3.8 27B                 | ✓    | 0.50  | 0.10         | 3.00   | 0.04–0.99        | 0.50–3.20         | 21        |
| Beam 1.0                    | ✓    | 0.40  | 0.04         | 1.20   | 0.24–0.40        | 0.98–1.20         | 6         |
| MiniMax-M3                  | ✓    | 0.30  | 0.06         | 1.20   | 0.22–0.60        | 0.88–2.40         | 12        |
| Kimi K2.7 Code              | ✓    | 0.95  | 0.19         | 4.00   | 0.56–1.90        | 3.00–8.00         | 11        |
| Inkling                     | ✓    | 1.00  | 0.17         | 4.05   | 1.00             | 4.05              | 1         |
| Muse Glimmer                | ✓    | 0.30  | 0.04         | 1.20   | 0.30–0.35        | 1.20–1.50         | 2         |
| Nemotron 3.5 Super VL       | ✓    | 0.225 | 0.05         | 0.575  | 0.06–0.30        | 0.35–1.25         | 9         |
| Nemotron 3.5 Lightning      | ✓    | 0.06  | 0.035        | 0.20   | 0.025–0.07       | 0.12–0.20         | 4         |
| LongCat 2.0                 | ✓    | 0.75  | 0.015        | 2.95   | 0.30             | 1.20              | 1         |
| Mistral Medium 3.5          | ✓    | 1.50  | 0.15         | 7.50   | 1.50–1.65        | 7.50–8.25         | 1         |
| Mistral Small 4             | ✓    | 0.15  | 0.015        | 0.60   | 0.15–0.17        | 0.60–0.66         | 1         |
| gpt-oss-120b                | ✓    | 0.15  | 0.10         | 0.60   | 0.03–0.35        | 0.15–0.95         | 23        |

Muse Wonder Full and K2 Horizon have no API price. Moonshot's high-speed
endpoint for Kimi K3.1 Pro costs twice the list price. Artificial Analysis has
no results for Muse Spark 1.3 Contributor.

## Sources

- Artificial Analysis model pages, one per effort level:
  `https://artificialanalysis.ai/models/<model>-<effort>`, where `<model>` is
  `gpt-6-1-sol`, `gpt-6-1-terra`, `gpt-6-luna`, `claude-opus-5-5`,
  `claude-sonnet-5-5`, or `claude-haiku-5-5`; `<effort>` is `non-reasoning`,
  `low`, `medium`, `high`, or `xhigh`; and the page without a suffix is max.
- Artificial Analysis model pages for the other labs, listed in the
  [model index](https://artificialanalysis.ai/models), for example
  [kimi-k3-1-pro](https://artificialanalysis.ai/models/kimi-k3-1-pro),
  [deepseek-v4-1-flash-1017](https://artificialanalysis.ai/models/deepseek-v4-1-flash-1017),
  and [mimo-v2-6-flash](https://artificialanalysis.ai/models/mimo-v2-6-flash).
- [Terminal-Bench 4.0](https://artificialanalysis.ai/evaluations/terminalbench-4-0)
- [FrontierCode 1.1](https://artificialanalysis.ai/evaluations/frontiercode)
- [Coding Agent Index](https://artificialanalysis.ai/agents/coding-agents)
- [Claude Code vs Codex](https://artificialanalysis.ai/agents/coding-agents/comparisons/claude-code-vs-codex)
- [Codex vs Opencode](https://artificialanalysis.ai/agents/coding-agents/comparisons/codex-vs-opencode)
- [Claude Code vs Opencode](https://artificialanalysis.ai/agents/coding-agents/comparisons/claude-code-vs-opencode)
- [Kimi Code CLI vs Opencode](https://artificialanalysis.ai/agents/coding-agents/comparisons/kimi-code-cli-vs-opencode)
- [DeepSeek Harness vs Opencode](https://artificialanalysis.ai/agents/coding-agents/comparisons/deepseek-harness-vs-opencode)
- [Muse Code vs Opencode](https://artificialanalysis.ai/agents/coding-agents/comparisons/muse-code-vs-opencode)
- [Mirror vs Opencode](https://artificialanalysis.ai/agents/coding-agents/comparisons/mirror-vs-opencode)
- [GPT-6.1 Terra pushes the cost efficiency frontier](https://artificialanalysis.ai/articles/gpt-6-1-terra-pushes-the-cost-efficiency-frontier)
- [Claude Opus 5.5 takes the top spot on the Intelligence Index](https://artificialanalysis.ai/articles/claude-opus-5-5)
- [Claude Sonnet 5.5 reaches #2 on the Intelligence Index](https://artificialanalysis.ai/articles/claude-sonnet-5-5)
- [Anthropic has released Claude Haiku 5.5](https://artificialanalysis.ai/articles/claude-haiku-5-5)
- [Anthropic: Claude Opus 5.5](https://www.anthropic.com/news/claude-opus-5-5)
- [Anthropic: Claude Sonnet 5.5](https://www.anthropic.com/news/claude-sonnet-5-5)
- [OpenRouter model list](https://openrouter.ai/api/v1/models), for each
  model's accepted effort levels, and its provider list,
  `https://openrouter.ai/api/v1/models/<id>/endpoints`, for each provider's
  prices
