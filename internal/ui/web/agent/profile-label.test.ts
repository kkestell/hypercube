import {expect, test} from 'bun:test';
import {displayName, modelName} from './profile-label.ts';

test('names use proper spelling and capitalization', () => {
  const cases: [string, string][] = [
    [displayName('xhigh'), 'Extra High'],
    [displayName('opencode'), 'OpenCode'],
    [modelName('haiku'), 'Haiku'],
    [modelName('gpt-6.1-sol'), 'GPT-6.1 Sol'],
    [modelName('gpt-6-luna'), 'GPT-6 Luna'],
    [modelName('opencode-go/deepseek-v4.1-flash'), 'DeepSeek V4.1 Flash'],
    [modelName('openrouter/x/m'), 'X M'],
    [modelName('some-model'), 'Some Model'],
  ];
  for (const [name, expected] of cases) expect(name).toBe(expected);
});
