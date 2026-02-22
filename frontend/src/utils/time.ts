export function defaultTimeRange() {
  const now = new Date();
  const sixHoursAgo = new Date(now.getTime() - 6 * 60 * 60 * 1000);
  const fmtDate = (d: Date) => d.toISOString().split('T')[0];
  const fmtTime = (d: Date) => d.toTimeString().slice(0, 5);
  return {
    startDate: fmtDate(sixHoursAgo),
    startTime: fmtTime(sixHoursAgo),
    endDate: fmtDate(now),
    endTime: fmtTime(now),
  };
}
