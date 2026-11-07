export const anonymousBootstrap = {
  initialized: true,
  authenticated: false,
  administrator: null,
  expires_at: null,
  csrf_token: "test-csrf",
};
export const authSession = {
  administrator: { id: "admin-test", username: "admin" },
  expires_at: "2099-01-01T00:00:00Z",
  csrf_token: "session-csrf",
};
