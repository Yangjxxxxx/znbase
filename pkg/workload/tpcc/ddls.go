// Copyright 2018  The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package tpcc

import (
	"fmt"

	"github.com/jackc/pgx"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

const (
	tpccWarehouseSchema = `(
		w_id        integer   not null ,
		w_name      varchar(10),
		w_street_1  varchar(20),
		w_street_2  varchar(20),
		w_city      varchar(20),
		w_state     char(2),
		w_zip       char(9),
		w_tax       decimal(4,4),
		w_ytd       decimal(12,2),
        primary key (w_id)%s
	)`
	tpccDistrictSchema = `(
		d_id         integer       not null,
		d_w_id       integer       not null,
		d_name       varchar(10),
		d_street_1   varchar(20),
		d_street_2   varchar(20),
		d_city       varchar(20),
		d_state      char(2),
		d_zip        char(9),
		d_tax        decimal(4,4),
		d_ytd        decimal(12,2),
		d_next_o_id  integer,
		primary key (d_w_id, d_id)%s
	)`
	tpccDistrictSchemaInterleave = ` interleave in parent warehouse (d_w_id)`
	tpccCustomerSchema           = `(
		c_id           integer        not null,
		c_d_id         integer        not null,
		c_w_id         integer        not null,
		c_first        varchar(16),
		c_middle       char(2),
		c_last         varchar(16),
		c_street_1     varchar(20),
		c_street_2     varchar(20),
		c_city         varchar(20),
		c_state        char(2),
		c_zip          char(9),
		c_phone        char(16),
		c_since        timestamp,
		c_credit       char(2),
		c_credit_lim   decimal(12,2),
		c_discount     decimal(4,4),
		c_balance      decimal(12,2),
		c_ytd_payment  decimal(12,2),
		c_payment_cnt  integer,
		c_delivery_cnt integer,
		c_data         varchar(500),
		primary key (c_w_id, c_d_id, c_id)%s,
		index customer_idx (c_w_id, c_d_id, c_last, c_first)%s
	)`
	tpccCustomerSchemaInterleave = ` interleave in parent district (c_w_id, c_d_id)`
	// No PK necessary for this table.
	tpccHistorySchema = `(
		rowid    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		h_c_id   integer,
		h_c_d_id integer,
		h_c_w_id integer,
		h_d_id   integer,
		h_w_id   integer,
		h_date   timestamp,
		h_amount decimal(6,2),
		h_data   varchar(24),
		index (h_w_id, h_d_id)%s,
		index (h_c_w_id, h_c_d_id, h_c_id)%s
	)`
	tpccOrderSchema = `(
		o_id         integer      not null,
		o_d_id       integer      not null,
		o_w_id       integer      not null,
		o_c_id       integer,
		o_entry_d    timestamp,
		o_carrier_id integer,
		o_ol_cnt     integer,
		o_all_local  integer,
		primary key (o_w_id, o_d_id, o_id DESC)%s,
		unique index order_idx (o_w_id, o_d_id, o_carrier_id, o_id)%s,
		index (o_w_id, o_d_id, o_c_id)%s
	)`
	tpccOrderSchemaInterleave = ` interleave in parent district (o_w_id, o_d_id)`
	tpccNewOrderSchema        = `(
		no_o_id  integer   not null,
		no_d_id  integer   not null,
		no_w_id  integer   not null,
		primary key (no_w_id, no_d_id, no_o_id)%s
	)`
	tpccNewOrderSchemaInterleave = ` interleave in parent "order" (no_w_id, no_d_id, no_o_id)`
	tpccItemSchema               = `(
		i_id     integer      not null,
		i_im_id  integer,
		i_name   varchar(24),
		i_price  decimal(5,2),
		i_data   varchar(50),
		primary key (i_id)
	)`
	tpccStockSchema = `(
		s_i_id       integer       not null,
		s_w_id       integer       not null,
		s_quantity   integer,
		s_dist_01    char(24),
		s_dist_02    char(24),
		s_dist_03    char(24),
		s_dist_04    char(24),
		s_dist_05    char(24),
		s_dist_06    char(24),
		s_dist_07    char(24),
		s_dist_08    char(24),
		s_dist_09    char(24),
		s_dist_10    char(24),
		s_ytd        integer,
		s_order_cnt  integer,
		s_remote_cnt integer,
		s_data       varchar(50),
		primary key (s_w_id, s_i_id)%s,
		index (s_i_id)
	)`
	tpccStockSchemaInterleave = ` interleave in parent warehouse (s_w_id)`
	tpccOrderLineSchema       = `(
		ol_o_id         integer   not null,
		ol_d_id         integer   not null,
		ol_w_id         integer   not null,
		ol_number       integer   not null,
		ol_i_id         integer   not null,
		ol_supply_w_id  integer,
		ol_delivery_d   timestamp,
		ol_quantity     integer,
		ol_amount       decimal(6,2),
		ol_dist_info    char(24),
		primary key (ol_w_id, ol_d_id, ol_o_id DESC, ol_number)%s,
		index order_line_fk (ol_supply_w_id, ol_i_id)%s
	)`
	tpccOrderLineSchemaInterleave = ` interleave in parent "order" (ol_w_id, ol_d_id, ol_o_id)`
)

func scatterRanges(db *pgx.ConnPool) {
	tables := []string{
		`customer`,
		`district`,
		`history`,
		`item`,
		`new_order`,
		`"order"`,
		`order_line`,
		`stock`,
		`warehouse`,
	}

	var g errgroup.Group
	for _, table := range tables {
		g.Go(func() error {
			sql := fmt.Sprintf(`ALTER TABLE %s SCATTER`, table)
			if _, err := db.Exec(sql); err != nil {
				return errors.Wrapf(err, "Couldn't exec %s", sql)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		panic(err)
	}
}
