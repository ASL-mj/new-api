import {
  getDurationTone,
  getFirstResponseTone,
  getTimingToneStyles,
} from './usageLogTiming';

describe('usage log timing tones', () => {
  test('uses the established total duration thresholds in seconds', () => {
    expect(getDurationTone(100)).toBe('success');
    expect(getDurationTone(101)).toBe('warning');
    expect(getDurationTone(299)).toBe('warning');
    expect(getDurationTone(300)).toBe('danger');
  });

  test('uses the established first response thresholds in milliseconds', () => {
    expect(getFirstResponseTone(2999)).toBe('success');
    expect(getFirstResponseTone(3000)).toBe('warning');
    expect(getFirstResponseTone(9999)).toBe('warning');
    expect(getFirstResponseTone(10000)).toBe('danger');
  });

  test('returns Semi theme variables for every timing tone', () => {
    expect(getTimingToneStyles('danger')).toEqual({
      color: 'var(--semi-color-danger)',
      backgroundColor: 'var(--semi-color-danger-light-default)',
      borderColor: 'var(--semi-color-danger-light-hover)',
    });
  });
});
