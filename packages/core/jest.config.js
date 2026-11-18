module.exports = {
  testEnvironment: 'node',
  transform: { '^.+\\.(t|j)sx?$': ['@swc/jest', { jsc: { transform: { react: { runtime: 'automatic' } } } }] },
  testMatch: ['**/__tests__/**/*.test.ts?(x)'],
};
