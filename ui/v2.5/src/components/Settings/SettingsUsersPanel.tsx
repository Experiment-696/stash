import React, { useState } from "react";
import { Alert, Button, Form, Modal, Table } from "react-bootstrap";
import * as GQL from "src/core/generated-graphql";

const PasswordResetModal: React.FC<{
  username: string;
  onCancel: () => void;
  onConfirm: (password: string) => Promise<void>;
}> = ({ username, onCancel, onConfirm }) => {
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const valid = password.length >= 8 && password === confirmation;

  return <Modal show onHide={onCancel} backdrop="static">
    <Modal.Header closeButton><Modal.Title>Reset password for {username}</Modal.Title></Modal.Header>
    <Modal.Body>
      <p>This revokes all existing sessions and API tokens. The user must change this temporary password after signing in.</p>
      <Form.Group controlId="reset-user-password">
        <Form.Label>Temporary password</Form.Label>
        <Form.Control type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.currentTarget.value)} />
      </Form.Group>
      <Form.Group controlId="reset-user-password-confirmation">
        <Form.Label>Confirm temporary password</Form.Label>
        <Form.Control type="password" autoComplete="new-password" value={confirmation} onChange={(e) => setConfirmation(e.currentTarget.value)} />
        <Form.Text muted>Use at least 8 characters; both entries must match.</Form.Text>
      </Form.Group>
    </Modal.Body>
    <Modal.Footer>
      <Button variant="secondary" onClick={onCancel}>Cancel</Button>
      <Button variant="danger" disabled={!valid} onClick={() => void onConfirm(password)}>Reset and revoke access</Button>
    </Modal.Footer>
  </Modal>;
};

export const SettingsUsersPanel: React.FC = () => {
  const { data: meData, loading: meLoading, error: meError } = GQL.useMeQuery({ fetchPolicy: "no-cache" });
  const isAdmin = meData?.me.role === "ADMIN" && meData.me.status === "ACTIVE";
  const { data, loading, error, refetch } = GQL.useUsersQuery({ skip: !isAdmin });
  const [auditOffset, setAuditOffset] = useState(0);
  const auditLimit = 25;
  const {
    data: auditData,
    loading: auditLoading,
    refetch: refetchAuditEvents,
  } = GQL.useAuditEventsQuery({
    variables: { limit: auditLimit, offset: auditOffset },
    skip: !isAdmin,
    fetchPolicy: "no-cache",
  });
  const [createUser] = GQL.useCreateUserMutation();
  const [updateAccess] = GQL.useUpdateUserAccessMutation();
  const [resetPassword] = GQL.useResetUserPasswordMutation();
  const [revokeSessions] = GQL.useRevokeUserSessionsMutation();
  const [revokeTokens] = GQL.useRevokeUserApiTokensMutation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("USER");
  const [failure, setFailure] = useState<string>();
  const [resetTarget, setResetTarget] = useState<{ id: string; username: string }>();

  if (meLoading) return <p>Loading users…</p>;
  if (meError || !isAdmin) return <Alert variant="danger">You do not have permission to manage users.</Alert>;
  if (loading) return <p>Loading users…</p>;
  if (error) return <Alert variant="danger">Unable to load users.</Alert>;

  async function run(action: () => Promise<unknown>) {
    setFailure(undefined);
    try {
      await action();
      await Promise.all([refetch(), refetchAuditEvents()]);
    } catch {
      setFailure("The user operation could not be completed.");
    }
  }

  return <div>
    <h2>Users</h2>
    {failure && <Alert variant="danger">{failure}</Alert>}
    <Form className="mb-4" onSubmit={(event) => {
      event.preventDefault();
      void run(async () => {
        await createUser({ variables: { input: { username, password, role } } });
        setUsername(""); setPassword(""); setRole("USER");
      });
    }}>
      <h3>Create user</h3>
      <Form.Control className="mb-2" aria-label="Username" placeholder="Username" value={username} onChange={(e) => setUsername(e.currentTarget.value)} />
      <Form.Control className="mb-2" aria-label="Password" placeholder="Password" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.currentTarget.value)} />
      <Form.Control className="mb-2" as="select" aria-label="Role" value={role} onChange={(e) => setRole(e.currentTarget.value)}>
        <option value="USER">User</option><option value="MODERATOR">Moderator</option><option value="ADMIN">Admin</option>
      </Form.Control>
      <Button type="submit" disabled={!username.trim() || password.length < 8}>Create user</Button>
    </Form>
    <Table responsive striped>
      <thead><tr><th>Username</th><th>Role</th><th>Status</th><th>Actions</th></tr></thead>
      <tbody>{data?.users.map((user) => <tr key={user.id}>
        <td>{user.username}</td>
        <td><Form.Control as="select" value={user.role} onChange={(e) => {
          const nextRole = e.currentTarget.value;
          if (!window.confirm(`Change ${user.username}'s role to ${nextRole}?`)) return;
          void run(() => updateAccess({ variables: { input: { id: user.id, role: nextRole, status: user.status } } }));
        }}><option value="USER">User</option><option value="MODERATOR">Moderator</option><option value="ADMIN">Admin</option></Form.Control></td>
        <td><Form.Control as="select" value={user.status} onChange={(e) => {
          const nextStatus = e.currentTarget.value;
          if (!window.confirm(`Change ${user.username}'s status to ${nextStatus}?`)) return;
          void run(() => updateAccess({ variables: { input: { id: user.id, role: user.role, status: nextStatus } } }));
        }}><option value="ACTIVE">Active</option><option value="DISABLED">Disabled</option><option value="PASSWORD_CHANGE_REQUIRED">Password change required</option></Form.Control></td>
        <td>
          <Button size="sm" className="mr-1" onClick={() => setResetTarget({ id: user.id, username: user.username })}>Reset password</Button>
          <Button size="sm" className="mr-1" onClick={() => window.confirm("Revoke all sessions for this user?") && void run(() => revokeSessions({ variables: { id: user.id } }))}>Revoke sessions</Button>
          <Button size="sm" onClick={() => window.confirm("Revoke all API tokens for this user?") && void run(() => revokeTokens({ variables: { id: user.id } }))}>Revoke tokens</Button>
        </td>
      </tr>)}</tbody>
    </Table>
    <h3>Security audit</h3>
    {auditLoading ? <p>Loading audit events…</p> : <>
      <Table responsive striped size="sm">
        <thead><tr><th>Time</th><th>Event</th><th>Actor</th><th>Target</th><th>Result</th></tr></thead>
        <tbody>{auditData?.auditEvents.map((event) => <tr key={event.id}>
          <td>{new Date(event.occurredAt).toLocaleString()}</td>
          <td>{event.eventType}</td>
          <td>{event.actorUserId ?? "—"}</td>
          <td>{event.targetType && event.targetId ? `${event.targetType} ${event.targetId}` : "—"}</td>
          <td>{event.result}</td>
        </tr>)}</tbody>
      </Table>
      <div className="d-flex justify-content-between mb-4">
        <Button disabled={auditOffset === 0} onClick={() => setAuditOffset(Math.max(0, auditOffset - auditLimit))}>Previous</Button>
        <Button disabled={(auditData?.auditEvents.length ?? 0) < auditLimit} onClick={() => setAuditOffset(auditOffset + auditLimit)}>Next</Button>
      </div>
    </>}
    {resetTarget && <PasswordResetModal
      username={resetTarget.username}
      onCancel={() => setResetTarget(undefined)}
      onConfirm={(temporaryPassword) => run(async () => {
        await resetPassword({ variables: { id: resetTarget.id, password: temporaryPassword } });
        setResetTarget(undefined);
      })}
    />}
  </div>;
};
