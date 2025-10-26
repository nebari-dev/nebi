import { useEffect, useState, useRef } from 'react';
import { useAuthStore } from '@/store/authStore';

const API_BASE_URL = import.meta.env.VITE_API_URL || '/api/v1';

export const useJobLogStream = (jobId: string, jobStatus: string) => {
  const [logs, setLogs] = useState<string>('');
  const [isStreaming, setIsStreaming] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);
  const token = useAuthStore((state) => state.token);

  useEffect(() => {
    // Only stream for running jobs
    if (jobStatus !== 'running' && jobStatus !== 'pending') {
      setIsStreaming(false);
      return;
    }

    if (!token) {
      console.error('No auth token available for log streaming');
      return;
    }

    setIsStreaming(true);

    // Build the SSE URL using the same base as axios client
    const url = `${API_BASE_URL}/jobs/${jobId}/logs/stream?token=${encodeURIComponent(token)}`;
    console.log('Connecting to SSE stream:', url);

    const eventSource = new EventSource(url);
    eventSourceRef.current = eventSource;

    eventSource.onopen = () => {
      console.log('SSE connection opened for job:', jobId);
    };

    eventSource.onmessage = (event) => {
      console.log('Received log data:', event.data);
      setLogs((prev) => prev + event.data);
    };

    eventSource.addEventListener('done', (event) => {
      console.log('SSE stream completed:', event);
      setIsStreaming(false);
      eventSource.close();
    });

    eventSource.onerror = (error) => {
      console.error('EventSource error:', error);
      console.error('EventSource readyState:', eventSource.readyState);
      setIsStreaming(false);
      eventSource.close();
    };

    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
    };
  }, [jobId, jobStatus, token]);

  return { logs, isStreaming };
};
