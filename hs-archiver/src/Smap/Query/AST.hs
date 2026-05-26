module Smap.Query.AST where

import Data.Text (Text)

data Query
    = Select SelectQuery
    | Set SetQuery
    | Delete DeleteQuery
    deriving (Show, Eq)

data SelectQuery = SelectQuery
    { selectFields :: [Field]
    , selectWhere  :: Maybe WhereClause
    , selectLimit  :: Maybe Int
    , selectStreamLimit :: Maybe Int
    } deriving (Show, Eq)

data Field
    = AllFields
    | UuidField
    | PathField
    | MetadataField Text
    | DataField DataRange
    deriving (Show, Eq)

data DataRange = DataRange
    { dataStart :: Maybe TimeExpr
    , dataEnd   :: Maybe TimeExpr
    , dataLimit :: Maybe Int
    } deriving (Show, Eq)

data TimeExpr
    = Now
    | AbsolutePosix Double
    | AbsoluteIso Text
    | Relative TimeExpr DiffTime
    deriving (Show, Eq)

data DiffTime = DiffTime
    { diffValue :: Integer
    , diffUnit  :: TimeUnit
    } deriving (Show, Eq)

data TimeUnit = Second | Minute | Hour | Day | Week
    deriving (Show, Eq)

data WhereClause
    = Has Text
    | Equals Text Text
    | Like Text Text
    | Tilde Text Text -- Regex
    | Contains Text Text
    | And WhereClause WhereClause
    | Or WhereClause WhereClause
    | Not WhereClause
    | In UuidList
    deriving (Show, Eq)

type UuidList = [Text] -- Or actual UUIDs

-- Placeholder for other query types
data SetQuery = SetQuery deriving (Show, Eq)
data DeleteQuery = DeleteQuery deriving (Show, Eq)
