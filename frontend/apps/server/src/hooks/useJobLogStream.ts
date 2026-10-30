import { useEffect, useState } from 'react';
import { getApiBaseUrl } from '@/lib/basePath';
import { getAccessToken } from '@/lib/oidc';

const API_BASE_URL = import.meta.env.VITE_API_URL || getApiBaseUrl();

export const useJobLogStream = (
  jobId: string,
  jobStatus: string,
  initialLogs: string = '',
) => {
  const [logs, setLogs] = useState<string>(initialLogs);
  const [isStreaming, setIsStreaming] = useState(false);

  useEffect(() => {
    // Only stream for jobs that may still emit logs.
    if (jobStatus !== 'running' && jobStatus !== 'pending') {
      setIsStreaming(false);
      return;
    }

    const abortController = new AbortController();
    setIsStreaming(true);

    const appendLogData = (data: string) => {
      setLogs((prev) => {
        const hasNewline = data.endsWith('\n');
        return prev + data + (hasNewline ? '' : '\n');
      });
    };

    const streamLogs = async () => {
      let buffer = '';
      let eventName = '';
      let dataLines: string[] = [];

      const resetEvent = () => {
        eventName = '';
        dataLines = [];
      };

      const finishEvent = () => {
        const data = dataLines.join('\n');
        if (eventName === 'done') {
          return true;
        }
        if (eventName === 'error') {
          console.error('SSE stream error:', data);
          return true;
        }
        if (dataLines.length > 0) {
          appendLogData(data);
        }
        resetEvent();
        return false;
      };

      const processLine = (line: string) => {
        if (line === '') {
          return finishEvent();
        }
        if (line.startsWith('event:')) {
          eventName = line.slice('event:'.length).trimStart();
          return false;
        }
        if (line.startsWith('data:')) {
          let data = line.slice('data:'.length);
          if (data.startsWith(' ')) {
            data = data.slice(1);
          }
          dataLines.push(data);
        }
        return false;
      };

      try {
        // Same credentials as apiClient: the IdP access token in team+OIDC
        // mode, none in local mode or when the server has auth disabled.
        const token = await getAccessToken();
        const headers: Record<string, string> = {
          Accept: 'text/event-stream',
        };
        if (token) {
          headers.Authorization = `Bearer ${token}`;
        }
        const response = await fetch(
          `${API_BASE_URL}/jobs/${jobId}/logs/stream`,
          {
            headers,
            signal: abortController.signal,
          },
        );

        if (!response.ok) {
          throw new Error(`Log stream failed with status ${response.status}`);
        }
        if (!response.body) {
          throw new Error('Log stream response body is not readable');
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();

        while (true) {
          const { value, done } = await reader.read();
          if (done) {
            break;
          }

          buffer += decoder.decode(value, { stream: true });
          let lineEnd = buffer.indexOf('\n');
          while (lineEnd !== -1) {
            let line = buffer.slice(0, lineEnd);
            if (line.endsWith('\r')) {
              line = line.slice(0, -1);
            }
            buffer = buffer.slice(lineEnd + 1);
            if (processLine(line)) {
              await reader.cancel();
              return;
            }
            lineEnd = buffer.indexOf('\n');
          }
        }

        buffer += decoder.decode();
        if (buffer !== '') {
          processLine(buffer);
        }
        if (dataLines.length > 0) {
          finishEvent();
        }
      } catch (error) {
        if (!abortController.signal.aborted) {
          console.error('SSE stream error:', error);
        }
      } finally {
        if (!abortController.signal.aborted) {
          setIsStreaming(false);
        }
      }
    };

    void streamLogs();

    return () => {
      abortController.abort();
    };
  }, [jobId, jobStatus]);

  return { logs, isStreaming };
};
