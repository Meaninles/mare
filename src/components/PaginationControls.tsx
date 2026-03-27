import { ChevronLeft, ChevronRight } from "lucide-react";
import { PAGE_SIZE_OPTIONS, type PaginationState } from "../hooks/usePagination";

export function PaginationControls<T>({
  pagination,
  itemLabel = "项"
}: {
  pagination: PaginationState<T>;
  itemLabel?: string;
}) {
  if (pagination.totalItems === 0) {
    return null;
  }

  return (
    <div className="pagination-bar">
      <div className="pagination-summary">
        <span>
          显示 {pagination.rangeStart}-{pagination.rangeEnd} / {pagination.totalItems} {itemLabel}
        </span>
      </div>

      <div className="pagination-actions">
        <label className="pagination-size">
          <span>每页</span>
          <select value={pagination.pageSize} onChange={(event) => pagination.setPageSize(Number(event.target.value))}>
            {PAGE_SIZE_OPTIONS.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
        </label>

        <span className="pagination-page">
          第 {pagination.page} / {pagination.pageCount} 页
        </span>

        <button type="button" className="ghost-button pagination-button" onClick={() => pagination.setPage(pagination.page - 1)} disabled={!pagination.canGoPrev}>
          <ChevronLeft size={14} />
          上一页
        </button>

        <button type="button" className="ghost-button pagination-button" onClick={() => pagination.setPage(pagination.page + 1)} disabled={!pagination.canGoNext}>
          下一页
          <ChevronRight size={14} />
        </button>
      </div>
    </div>
  );
}
