import React, { useRef, useState } from "react";
import { Alert, Button, Modal, Spinner } from "react-bootstrap";
import { ButtonVariant } from "react-bootstrap/types";

interface IProps {
  testID: string;
  title: string;
  description: React.ReactNode;
  triggerLabel: React.ReactNode;
  confirmLabel: string;
  onConfirm: () => Promise<void>;
  onError?: (error: unknown) => void;
  disabled?: boolean;
  triggerVariant?: ButtonVariant;
  triggerSize?: "sm" | "lg";
  triggerClassName?: string;
}

export const CamModelConfirmedAction: React.FC<IProps> = ({
  testID,
  title,
  description,
  triggerLabel,
  confirmLabel,
  onConfirm,
  onError,
  disabled,
  triggerVariant = "warning",
  triggerSize,
  triggerClassName,
}) => {
  const [show, setShow] = useState(false);
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<string>();
  const locked = useRef(false);
  const confirm = async () => {
    if (locked.current) return;
    locked.current = true;
    setPending(true);
    setFailure(undefined);
    try {
      await onConfirm();
      setShow(false);
    } catch (error) {
      setFailure(
        error instanceof Error ? error.message : "The action failed. Try again."
      );
      onError?.(error);
    } finally {
      locked.current = false;
      setPending(false);
    }
  };
  return (
    <>
      <Button
        type="button"
        variant={triggerVariant}
        size={triggerSize}
        className={triggerClassName}
        disabled={disabled || pending}
        data-testid={`${testID}-trigger`}
        onClick={() => {
          setFailure(undefined);
          setShow(true);
        }}
      >
        {triggerLabel}
      </Button>
      <Modal
        animation={false}
        show={show}
        onHide={() => !pending && setShow(false)}
        aria-label={title}
        aria-modal
        data-testid={`${testID}-dialog`}
        role="dialog"
        keyboard={!pending}
        backdrop="static"
        autoFocus
        enforceFocus
        restoreFocus
      >
        <Modal.Header>
          <Modal.Title>{title}</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          <div data-testid={`${testID}-description`}>{description}</div>
          {failure && (
            <Alert
              className="mt-3 mb-0"
              variant="danger"
              role="alert"
              aria-live="assertive"
              data-testid={`${testID}-error`}
            >
              {failure}
            </Alert>
          )}
        </Modal.Body>
        <Modal.Footer>
          <Button
            type="button"
            variant="secondary"
            disabled={pending}
            onClick={() => {
              setFailure(undefined);
              setShow(false);
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="danger"
            disabled={pending}
            onClick={() => void confirm()}
          >
            {pending ? (
              <>
                <Spinner animation="border" role="status" size="sm" />{" "}
                Confirming…
              </>
            ) : (
              confirmLabel
            )}
          </Button>
        </Modal.Footer>
      </Modal>
    </>
  );
};
