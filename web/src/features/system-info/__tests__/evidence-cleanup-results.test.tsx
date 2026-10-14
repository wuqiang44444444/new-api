/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test } from "vitest";

import { SystemTasksTable } from "../components/system-tasks-table";

test("history exposes evidence cleanup results including partial failures", async () => {
  render(
    <SystemTasksTable
      tasks={[
        {
          id: 1,
          task_id: "historical-cleanup",
          type: "task_request_evidence_retention",
          status: "failed",
          created_at: 100,
          updated_at: 200,
          error: "delete_failed",
          result: {
            scanned: 3,
            cleaned_indexes: 2,
            deleted_files: 4,
            freed_bytes: 202,
            failed_items: 1,
            missing_files: 0,
            unattributed_files: 0,
            deleted_events: 9,
            deleted_indexes: 2,
            deleted_access_logs: 3,
            deleted_history: 1,
            database_failures: 0,
            error_category: "delete_failed",
          },
        },
      ]}
    />,
  );
  expect(screen.getByText("Request evidence cleanup")).toBeVisible();
  await userEvent.click(
    screen.getByRole("button", { name: "Cleanup results" }),
  );
  const dialog = await screen.findByRole("dialog", {
    name: "Evidence cleanup results",
  });
  expect(
    within(dialog).getByText("Failed to delete evidence objects"),
  ).toBeVisible();
  expect(
    within(
      within(dialog).getByRole("group", { name: "Bytes removed" }),
    ).getByText("202"),
  ).toBeVisible();
  expect(
    within(
      within(dialog).getByRole("group", { name: "Event rows removed" }),
    ).getByText("9"),
  ).toBeVisible();
  expect(
    within(
      within(dialog).getByRole("group", { name: "Index rows removed" }),
    ).getByText("2"),
  ).toBeVisible();
});
