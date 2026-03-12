import { create } from 'zustand';

interface DeviceCodeState {
  connecting: boolean;
  approvalUrl: string | null;
  error: string | null;
  setConnecting: (v: boolean) => void;
  setApprovalUrl: (url: string | null) => void;
  setError: (err: string | null) => void;
  reset: () => void;
}

export const useDeviceCodeStore = create<DeviceCodeState>()((set) => ({
  connecting: false,
  approvalUrl: null,
  error: null,
  setConnecting: (v) => set({ connecting: v }),
  setApprovalUrl: (url) => set({ approvalUrl: url }),
  setError: (err) => set({ error: err }),
  reset: () => set({ connecting: false, approvalUrl: null, error: null }),
}));
