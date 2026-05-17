export type RightSidebarLayoutDirection = "ltr" | "rtl"

export function getRightSidebarPanelDefaultSizes(showRightSidebar: boolean, rightSidebarSizePercent: number) {
  const rightSidebar = showRightSidebar ? rightSidebarSizePercent : 0
  return {
    workspace: 100 - rightSidebar,
    rightSidebar,
  }
}

export function getOrderedRightSidebarLayout(
  workspace: number,
  rightSidebar: number,
  direction: RightSidebarLayoutDirection
) {
  return direction === "rtl"
    ? { rightSidebar, workspace }
    : { workspace, rightSidebar }
}

