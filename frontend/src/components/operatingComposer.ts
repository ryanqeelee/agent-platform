export interface OperatingComposer {
  draft: string;
  disabled: boolean;
  running: boolean;
  cancelling: boolean;
  uploadPending: boolean;
  uploadAvailable: boolean;
  attachments: Array<{
    id: string;
    name: string;
    size: number;
    status: 'ready' | 'uploading' | 'error';
    error?: string;
  }>;
  setDraft: (text: string) => void;
  send: (text: string) => void;
  stop: () => void;
  upload: (files: File[]) => void;
  remove: (id: string) => void;
}

export type OperatingStatus =
  | 'idle'
  | 'running'
  | 'completed'
  | 'cancelled'
  | 'failed'
  | 'incomplete';

export const canSendOperatingDraft = (
  composer: OperatingComposer,
  text = composer.draft,
): boolean => (
  !composer.disabled
  && !composer.running
  && !composer.cancelling
  && !composer.uploadPending
  && text.trim().length > 0
);

export const sendOperatingDraft = (
  composer: OperatingComposer,
  text = composer.draft,
): boolean => {
  if (!canSendOperatingDraft(composer, text)) return false;
  composer.send(text);
  return true;
};

export const isOperatingTerminalStatus = (status?: OperatingStatus): boolean => (
  status === 'completed'
  || status === 'cancelled'
  || status === 'failed'
  || status === 'incomplete'
);
