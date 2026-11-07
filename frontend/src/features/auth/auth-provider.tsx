import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createContext, useCallback, useContext, useEffect } from "react";

import { sessionExpiredEvent } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";

import type {
  AuthBootstrap,
  AuthSession,
  InitializeRequest,
  LoginRequest,
} from "./api";
import * as api from "./api";

export const authKey = ["auth", "bootstrap"] as const;

type AuthContextValue = {
  data: AuthBootstrap | undefined;
  error: Error | null;
  isPending: boolean;
  retry: () => Promise<unknown>;
  signIn: (input: LoginRequest) => Promise<void>;
  initialize: (input: InitializeRequest) => Promise<void>;
  signOut: () => Promise<void>;
};
const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const bootstrap = useQuery({
    queryKey: authKey,
    queryFn: ({ signal }) => api.getBootstrap(signal),
    retry: false,
    staleTime: 30_000,
    gcTime: 0,
    refetchOnWindowFocus: "always",
    refetchOnReconnect: "always",
  });
  const { refetch } = bootstrap;

  const clearProtected = useCallback(async () => {
    await queryClient.cancelQueries();
    queryClient.removeQueries({
      predicate: (query) => query.queryKey[0] !== "auth",
    });
    queryClient.getMutationCache().clear();
  }, [queryClient]);

  const refreshSession = useCallback(async () => {
    await clearProtected();
    // Remove stale identity before an offline/revoked session can render protected UI.
    queryClient.setQueryData<AuthBootstrap>(authKey, (old) =>
      old
        ? {
            ...old,
            authenticated: false,
            administrator: null,
            expires_at: null,
          }
        : old,
    );
    await refetch();
  }, [clearProtected, queryClient, refetch]);

  useEffect(() => {
    const handleExpired = () => {
      void refreshSession();
    };
    window.addEventListener(sessionExpiredEvent, handleExpired);
    return () => window.removeEventListener(sessionExpiredEvent, handleExpired);
  }, [refreshSession]);

  const expiry = bootstrap.data?.expires_at;
  useEffect(() => {
    if (!expiry) return;
    let timer: ReturnType<typeof setTimeout>;
    const schedule = () => {
      const remaining = Date.parse(expiry) - Date.now();
      if (remaining <= 0) {
        void refreshSession();
        return;
      }
      // Long lifetimes must not overflow the browser's signed 32-bit timer.
      timer = setTimeout(schedule, Math.min(remaining, 6 * 60 * 60 * 1000));
    };
    schedule();
    return () => clearTimeout(timer);
  }, [expiry, refreshSession]);

  useEffect(() => {
    if (bootstrap.data && !bootstrap.data.authenticated) {
      void queryClient.cancelQueries({
        predicate: (query) => query.queryKey[0] !== "auth",
      });
      queryClient.removeQueries({
        predicate: (query) => query.queryKey[0] !== "auth",
      });
    }
  }, [bootstrap.data, queryClient]);

  async function commitSession(session: AuthSession) {
    await clearProtected();
    queryClient.setQueryData<AuthBootstrap>(authKey, {
      initialized: true,
      authenticated: true,
      ...session,
    });
  }

  function csrfToken() {
    if (!bootstrap.data)
      throw new ApiError("setup_required", "请刷新页面后重试。");
    return bootstrap.data.csrf_token;
  }

  async function recoverTransition(error: unknown): Promise<never> {
    if (
      error instanceof ApiError &&
      ["csrf_invalid", "already_initialized", "setup_required"].includes(
        error.code,
      )
    ) {
      await refetch();
    }
    throw error;
  }

  return (
    <AuthContext.Provider
      value={{
        data: bootstrap.data,
        error: bootstrap.error,
        isPending: bootstrap.isPending,
        retry: refetch,
        signIn: async (input) => {
          try {
            await commitSession(await api.login(input, csrfToken()));
          } catch (error) {
            await recoverTransition(error);
          }
        },
        initialize: async (input) => {
          try {
            await commitSession(await api.initialize(input, csrfToken()));
          } catch (error) {
            await recoverTransition(error);
          }
        },
        signOut: async () => {
          try {
            await api.logout(csrfToken());
          } catch (error) {
            if (!(error instanceof ApiError && error.status === 401)) {
              await recoverTransition(error);
            }
          }
          await refreshSession();
        },
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) throw new Error("AuthProvider is required");
  return context;
}
