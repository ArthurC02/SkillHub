import { expect, test } from 'claude-code/testing'
import { spawnProblem } from '../hooks/register'

const PARENT = 'claude-opus-5-5'

test('a subagent with no model and no role is refused', () => {
  expect(spawnProblem(undefined, 'general-purpose', PARENT)).toMatch(/name a model/)
})

test('a repository role without a model is allowed, its frontmatter pins one', () => {
  expect(spawnProblem(undefined, 'skillhub-writer', PARENT)).toBeUndefined()
})

test('a subagent on the dispatcher\'s own family is refused', () => {
  expect(spawnProblem('opus', 'general-purpose', PARENT)).toMatch(/dispatcher's own tier/)
})

test('a subagent one tier below the dispatcher is allowed', () => {
  expect(spawnProblem('sonnet', 'general-purpose', PARENT)).toBeUndefined()
})

test('fable is refused even under a fable-less dispatcher', () => {
  expect(spawnProblem('fable', 'general-purpose', 'claude-sonnet-5-5')).toMatch(/dispatcher's own tier/)
})

test('opus is allowed when the dispatcher is fable', () => {
  expect(spawnProblem('opus', 'Explore', 'claude-fable-5-1')).toBeUndefined()
})
