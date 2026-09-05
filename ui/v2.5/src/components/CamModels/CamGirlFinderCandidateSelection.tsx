import React from "react";
import { Badge, Card, Form } from "react-bootstrap";
import { ExternalLink } from "../Shared/ExternalLink.js";
import type { CamGirlFinderCandidate } from "../../core/generated-graphql.js";

export const CamGirlFinderCandidateSelection: React.FC<{
  items: CamGirlFinderCandidate[];
  selected: string[];
  setSelected: React.Dispatch<React.SetStateAction<string[]>>;
}> = ({ items, selected, setSelected }) => (
  <>
    {items.map((item) => (
      <Card body className="mb-2" key={item.evidenceKey}>
        {item.imageURL && (
          <img
            src={item.imageURL}
            alt=""
            className="mr-3"
            style={{ width: 72, height: 72, objectFit: "cover" }}
          />
        )}
        <Form.Check
          inline
          type="checkbox"
          aria-label={"Select " + item.platform + " " + item.username}
          checked={selected.includes(item.evidenceKey)}
          onChange={(event) => {
            const { checked } = event.currentTarget;
            const { evidenceKey } = item;
            setSelected((current) =>
              checked
                ? current.includes(evidenceKey)
                  ? current
                  : [...current, evidenceKey]
                : current.filter((key) => key !== evidenceKey)
            );
          }}
        />
        <Badge variant="secondary">{item.platform}</Badge>{" "}
        <strong>{item.username}</strong>
        <div>Observed {new Date(item.observedAt).toLocaleString()}</div>
        {item.sourceURL && (
          <ExternalLink href={item.sourceURL}>
            Open profile
          </ExternalLink>
        )}
      </Card>
    ))}
  </>
);
