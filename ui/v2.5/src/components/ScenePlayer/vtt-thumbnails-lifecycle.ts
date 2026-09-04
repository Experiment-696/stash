export function isSuccessfulVttResponse(status: number): boolean {
  return status >= 200 && status < 300;
}

export function canApplyVttLoad(
  loadGeneration: number,
  currentGeneration: number,
  playerDisposed: boolean
): boolean {
  return loadGeneration === currentGeneration && !playerDisposed;
}
