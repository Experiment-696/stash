import React from "react";
import { Alert, Button, Form, Spinner } from "react-bootstrap";
import type { CamShowSortMode } from "../../core/generated-graphql";

const favoriteSortParameter = "favorite_models_first";
export const camShowSortModes = {
  Default: "DEFAULT" as CamShowSortMode,
  FavoriteModelsFirst: "FAVORITE_MODELS_FIRST" as CamShowSortMode,
};

export function camShowSortFromSearch(search: string): CamShowSortMode {
  const value = new URLSearchParams(search).get("sort");
  return value === favoriteSortParameter
    ? camShowSortModes.FavoriteModelsFirst
    : camShowSortModes.Default;
}

export function camShowSortSearch(
  search: string,
  sort: CamShowSortMode
): string {
  const query = new URLSearchParams(search);
  if (sort === camShowSortModes.FavoriteModelsFirst) {
    query.set("sort", favoriteSortParameter);
  } else {
    query.delete("sort");
  }
  const encoded = query.toString();
  return encoded ? `?${encoded}` : "";
}

export function uniqueCamShows<T extends { id: string }>(shows: T[]): T[] {
  const seen = new Set<string>();
  return shows.filter((show) => {
    if (seen.has(show.id)) return false;
    seen.add(show.id);
    return true;
  });
}

export const CamShowSortControl: React.FC<{
  sort: CamShowSortMode;
  onChange: (sort: CamShowSortMode) => void;
  loading: boolean;
  error?: string;
}> = ({ sort, onChange, loading, error }) => (
  <>
    <Form.Group controlId="cam-show-sort" className="mb-3">
      <Form.Label>Sort Shows</Form.Label>
      <div className="d-flex align-items-center">
        <Form.Control
          as="select"
          aria-label="Sort Shows"
          value={sort}
          disabled={loading}
          onChange={(event) =>
            onChange(event.currentTarget.value as CamShowSortMode)
          }
        >
          <option value={camShowSortModes.Default}>Newest Show date</option>
          <option value={camShowSortModes.FavoriteModelsFirst}>
            Favorite Models first
          </option>
        </Form.Control>
        <Button
          className="ml-2"
          variant="secondary"
          disabled={loading || sort === camShowSortModes.Default}
          onClick={() => onChange(camShowSortModes.Default)}
        >
          Reset sort
        </Button>
      </div>
      <Form.Text muted>
        Favorite Models uses only favorites from your signed-in account.
      </Form.Text>
    </Form.Group>
    {loading && (
      <div role="status" className="mb-3">
        <Spinner animation="border" size="sm" /> Updating Shows…
      </div>
    )}
    {error && (
      <Alert variant="danger" role="alert">
        {error}
      </Alert>
    )}
  </>
);
