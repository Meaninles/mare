import { useEffect, useMemo, useState } from "react";

export const PAGE_SIZE_OPTIONS = [10, 20, 50, 100] as const;

export type PaginationState<T> = {
  page: number;
  pageSize: number;
  totalItems: number;
  pageCount: number;
  rangeStart: number;
  rangeEnd: number;
  pagedItems: T[];
  canGoPrev: boolean;
  canGoNext: boolean;
  setPage: (page: number) => void;
  setPageSize: (pageSize: number) => void;
};

export function usePagination<T>(items: T[], initialPageSize = 20): PaginationState<T> {
  const normalizedInitialPageSize = PAGE_SIZE_OPTIONS.includes(initialPageSize as (typeof PAGE_SIZE_OPTIONS)[number])
    ? initialPageSize
    : 20;

  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(normalizedInitialPageSize);

  const totalItems = items.length;
  const pageCount = Math.max(1, Math.ceil(totalItems / pageSize));

  useEffect(() => {
    setPage((current) => Math.min(current, pageCount));
  }, [pageCount]);

  const pagedItems = useMemo(() => {
    const startIndex = (page - 1) * pageSize;
    return items.slice(startIndex, startIndex + pageSize);
  }, [items, page, pageSize]);

  const rangeStart = totalItems === 0 ? 0 : (page - 1) * pageSize + 1;
  const rangeEnd = totalItems === 0 ? 0 : Math.min(page * pageSize, totalItems);

  return {
    page,
    pageSize,
    totalItems,
    pageCount,
    rangeStart,
    rangeEnd,
    pagedItems,
    canGoPrev: page > 1,
    canGoNext: page < pageCount,
    setPage: (nextPage) => {
      const normalizedPage = Math.max(1, Math.min(nextPage, pageCount));
      setPage(normalizedPage);
    },
    setPageSize: (nextPageSize) => {
      const normalizedPageSize = PAGE_SIZE_OPTIONS.includes(nextPageSize as (typeof PAGE_SIZE_OPTIONS)[number])
        ? nextPageSize
        : normalizedInitialPageSize;
      setPage(1);
      setPageSize(normalizedPageSize);
    }
  };
}
