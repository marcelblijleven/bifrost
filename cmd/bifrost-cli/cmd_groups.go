package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func groupsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "groups", Short: "Manage groups"}
	cmd.AddCommand(groupsListCmd(), groupsCreateCmd(), groupsRenameCmd(), groupsDeleteCmd(), groupsMembersCmd())
	return cmd
}

func groupsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			groups, err := c.ListGroups(context.Background())
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				printJSON(groups)
				return nil
			}
			if len(groups) == 0 {
				fmt.Println("No groups.")
				return nil
			}
			rows := make([][]string, len(groups))
			for i, g := range groups {
				rows[i] = []string{g.ID[:8], g.Name, g.CreatedAt.Format("2006-01-02")}
			}
			printTable([]string{"ID", "NAME", "CREATED"}, rows)
			return nil
		},
	}
}

func groupsCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			g, err := c.CreateGroup(context.Background(), args[0])
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				printJSON(g)
				return nil
			}
			fmt.Printf(green+"✓"+reset+" Created group %s  id: %s\n", g.Name, g.ID)
			return nil
		},
	}
}

func groupsRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <id> <name>",
		Short: "Rename a group",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			g, err := c.UpdateGroup(context.Background(), args[0], args[1])
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				printJSON(g)
				return nil
			}
			fmt.Printf(green+"✓"+reset+" Renamed to %s\n", g.Name)
			return nil
		},
	}
}

func groupsDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				fmt.Printf("Delete group %s? [y/N] ", args[0])
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != "y" && confirm != "Y" {
					fmt.Println("Cancelled.")
					return nil
				}
			}
			c, err := resolveClient()
			if err != nil {
				return err
			}
			if err := c.DeleteGroup(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println(green + "✓" + reset + " Deleted.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func groupsMembersCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "members", Short: "Manage group members"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list <group-id>",
			Short: "List group members",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := resolveClient()
				if err != nil {
					return err
				}
				members, err := c.ListGroupMembers(context.Background(), args[0])
				if err != nil {
					return err
				}
				if flagOutput == "json" {
					printJSON(members)
					return nil
				}
				if len(members) == 0 {
					fmt.Println("No members.")
					return nil
				}
				rows := make([][]string, len(members))
				for i, u := range members {
					rows[i] = []string{u.ID[:8], u.Email}
				}
				printTable([]string{"ID", "EMAIL"}, rows)
				return nil
			},
		},
		&cobra.Command{
			Use:   "add <group-id> <user-id>",
			Short: "Add a user to the group",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := resolveClient()
				if err != nil {
					return err
				}
				if err := c.AddGroupMember(context.Background(), args[0], args[1]); err != nil {
					return err
				}
				fmt.Println(green + "✓" + reset + " Member added.")
				return nil
			},
		},
		&cobra.Command{
			Use:   "remove <group-id> <user-id>",
			Short: "Remove a user from the group",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := resolveClient()
				if err != nil {
					return err
				}
				if err := c.RemoveGroupMember(context.Background(), args[0], args[1]); err != nil {
					return err
				}
				fmt.Println(green + "✓" + reset + " Member removed.")
				return nil
			},
		},
	)
	return cmd
}
