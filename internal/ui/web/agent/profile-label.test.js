import {expect, test} from 'bun:test';
import {profileLabel} from './profile-label.js';

test('profile labels use proper names and capitalization', () => {
  const cases = [
    [{harness: 'claude', model: 'haiku', effort: 'low'}, 'Claude · Haiku · Low'],
    [{harness: 'claude', model: 'opus', effort: 'xhigh'}, 'Claude · Opus · Extra High'],
    [{harness: 'codex', model: 'gpt-6.1-sol', effort: 'high'}, 'Codex · GPT-6.1 Sol · High'],
    [{harness: 'codex', model: 'gpt-6-luna', effort: 'medium'}, 'Codex · GPT-6 Luna · Medium'],
    [{harness: 'opencode', model: 'opencode-go/deepseek-v4.1-flash', effort: 'max'}, 'OpenCode · DeepSeek V4.1 Flash · Max'],
    [{harness: 'pi', model: 'openrouter/x/m', effort: 'high'}, 'Pi · X M · High'],
    [{harness: 'pi', model: 'openrouter/y/m', effort: 'high'}, 'Pi · Y M · High'],
    [{harness: 'pi', model: 'some-model', effort: 'low'}, 'Pi · Some Model · Low'],
  ];
  for (const [profile, label] of cases) expect(profileLabel(profile)).toBe(label);
});
