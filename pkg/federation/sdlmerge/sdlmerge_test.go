package sdlmerge

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/TykTechnologies/graphql-go-tools/pkg/astprinter"
	"github.com/TykTechnologies/graphql-go-tools/pkg/astvisitor"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
)

type composeVisitor []Visitor

func (c composeVisitor) Register(walker *astvisitor.Walker) {
	for _, visitor := range c {
		visitor.Register(walker)
	}
}

var run = func(t *testing.T, visitor Visitor, operation, expectedOutput string) {

	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(expectedOutput)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)

	visitor.Register(&walker)

	walker.Walk(&operationDocument, nil, &report)

	if report.HasErrors() {
		t.Fatal(report.Error())
	}

	got := mustString(astprinter.PrintStringIndent(&operationDocument, nil, " "))
	want := mustString(astprinter.PrintStringIndent(&expectedOutputDocument, nil, " "))

	assert.Equal(t, want, got)
}

var runAndExpectError = func(t *testing.T, visitor Visitor, operation, expectedError string) {
	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)

	visitor.Register(&walker)

	walker.Walk(&operationDocument, nil, &report)

	var got string
	if report.HasErrors() {
		got = report.Error()
	}

	assert.Equal(t, expectedError, got)
}

func runMany(t *testing.T, operation, expectedOutput string, visitors ...Visitor) {
	run(t, composeVisitor(visitors), operation, expectedOutput)
}

func mustString(str string, err error) string {
	if err != nil {
		panic(err)
	}
	return str
}

func TestMergeSDLs(t *testing.T) {
	runMergeTest := func(expectedSchema string, sdls ...string) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			got, err := MergeSDLs(sdls...)
			if err != nil {
				t.Fatal(err)
			}

			expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(expectedSchema)
			want := mustString(astprinter.PrintString(&expectedOutputDocument, nil))

			assert.Equal(t, want, got)
		}
	}

	runMergeTestAndExpectError := func(expectedError string, sdls ...string) func(t *testing.T) {
		return func(t *testing.T) {
			_, err := MergeSDLs(sdls...)

			assert.Equal(t, expectedError, err.Error())
		}
	}

	t.Run("should merge all sdls successfully", runMergeTest(
		federatedSchema,
		accountSchema, productSchema, reviewSchema, likeSchema, disLikeSchema, paymentSchema, onlinePaymentSchema, classicPaymentSchema,
	))

	t.Run("should merge product and review sdl and leave `extend type User` in the schema", runMergeTest(
		productAndReviewFederatedSchema,
		productSchema, reviewSchema,
	))

	t.Run("should merge product and extends directives sdl and leave the type extension definition in the schema", runMergeTest(
		productAndExtendsDirectivesFederatedSchema,
		productSchema, extendsDirectivesSchema,
	))

	t.Run("Non-identical duplicate enums should return an error", runMergeTestAndExpectError(
		NonIdenticalSharedTypeMergeErrorMessage("Satisfaction"),
		productSchema, negativeTestingLikeSchema,
	))

	t.Run("Non-identical duplicate unions should return an error", runMergeTestAndExpectError(
		NonIdenticalSharedTypeMergeErrorMessage("AlphaNumeric"),
		accountSchema, negativeTestingReviewSchema,
	))

	t.Run("Entity duplicates should return an error", runMergeTestAndExpectError(
		DuplicateEntityMergeErrorMessage("User"),
		accountSchema, negativeTestingAccountSchema,
	))
}

const (
	accountSchema = `
		extend type Query {
			me: User
		}

		union AlphaNumeric = Int | String | Float

		scalar DateTime

		scalar CustomScalar

		type User @key(fields: "id") {
			id: ID!
			username: String!
			created: DateTime!
			reputation: CustomScalar!
		}

		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}
	`

	negativeTestingAccountSchema = `
		extend type Query {
			me: User
		}

		union AlphaNumeric = Int | String | Float

		scalar DateTime

		scalar CustomScalar

		type User {
			id: ID!
			username: String!
			created: DateTime!
			reputation: CustomScalar!
		}

		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}
	`

	productSchema = `
		enum Satisfaction {
			UNHAPPY,
			HAPPY,
			NEUTRAL,
		}

		scalar CustomScalar

		extend type Query {
			topProducts(first: Int = 5): [Product]
		}

		enum Department {
			COSMETICS,
			ELECTRONICS,
			GROCERIES,
		}

		interface ProductInfo {
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}

		scalar BigInt
		
		type Product implements ProductInfo @key(fields: "upc") {
			upc: String!
			name: String!
			price: Int!
			worth: BigInt!
			reputation: CustomScalar!
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}

		union AlphaNumeric = Int | String | Float
	`
	reviewSchema = `
		scalar DateTime

		input ReviewInput {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}

		type Review {
			id: ID!
			created: DateTime!
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}
		
		type Query {
			getReview(id: ID!): Review
		}

		type Mutation {
			createReview(input: ReviewInput): Review
			updateReview(id: ID!, input: ReviewInput): Review
		}
		
		enum Department {
			GROCERIES,
			COSMETICS,
			ELECTRONICS,
		}

		extend type User @key(fields: "id") {
			id: ID! @external
			reviews: [Review]
		}

		extend type Product @key(fields: "upc") {
			upc: String! @external
			name: String! @external
			reviews: [Review] @requires(fields: "name")
			sales: BigInt!
		}

		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		extend type Subscription {
			review: Review!
		}

		interface ProductInfo {
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}
	`

	negativeTestingReviewSchema = `
		scalar DateTime

		input ReviewInput {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}

		type Review {
			id: ID!
			created: DateTime!
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}
		
		type Query {
			getReview(id: ID!): Review
		}

		type Mutation {
			createReview(input: ReviewInput): Review
			updateReview(id: ID!, input: ReviewInput): Review
		}

		interface ProductInfo {
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}
		
		enum Department {
			COSMETICS,
			ELECTRONICS,
			GROCERIES,
		}
		
		extend type User @key(fields: "id") {
			id: ID! @external
			reviews: [Review]
		}

		extend type Product @key(fields: "upc") {
			upc: String! @external
			name: String! @external
			reviews: [Review] @requires(fields: "name")
			sales: BigInt!
		}

		union AlphaNumeric = BigInt | String
		
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		extend type Subscription {
			review: Review!
		}
	`

	likeSchema = `
		scalar DateTime

		type Like @key(fields: "id") {
			id: ID!
			productId: ID!
			userId: ID!
			date: DateTime!
		}
		
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		type Query {
			likesCount(productID: ID!): Int!
			likes(productID: ID!): [Like]!
		}
	`
	negativeTestingLikeSchema = `
		scalar DateTime

		type Like @key(fields: "id") {
			id: ID!
			productId: ID!
			userId: ID!
			date: DateTime!
		}
		
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
			DEVASTATED,
		}

		type Query {
			likesCount(productID: ID!): Int!
			likes(productID: ID!): [Like]!
		}
	`

	disLikeSchema = `
		type Like @key(fields: "id") @extends {
			id: ID! @external
			isDislike: Boolean!
		}

		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

	`
	paymentSchema = `
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		interface PaymentType {
			name: String!
		}
	`
	onlinePaymentSchema = `
		extend enum Satisfaction {
			UNHAPPY
		}

		scalar DateTime

		union AlphaNumeric = Int | String | Float

		scalar BigInt

		interface PaymentType @extends {
			email: String!
			date: DateTime!
			amount: BigInt!
		}
		
		extend union AlphaNumeric = Float

		enum Satisfaction {
			HAPPY
			NEUTRAL
		}
	`
	classicPaymentSchema = `
		union AlphaNumeric = Int | String | Float

		scalar CustomScalar

		extend interface PaymentType {
			number: String!
			reputation: CustomScalar!
		}
	`
	extendsDirectivesSchema = `
		scalar DateTime

		type Comment {
			body: String!
			author: User!
			created: DateTime!
		}

		type User @extends @key(fields: "id") {
			id: ID! @external
			comments: [Comment]
		}

		union AlphaNumeric = Int | String | Float

		interface PaymentType @extends {
			name: String!
		}
	`
	federatedSchema = `
		type Query {
			me: User
			topProducts(first: Int = 5): [Product]
			getReview(id: ID!): Review
			likesCount(productID: ID!): Int!
			likes(productID: ID!): [Like]!
		}

		type Mutation {
			createReview(input: ReviewInput): Review
			updateReview(id: ID!, input: ReviewInput): Review
		}
		
		type Subscription {
			review: Review!
		}

		union AlphaNumeric = Int | String | Float

		scalar DateTime

		scalar CustomScalar
		
		type User {
			id: ID!
			username: String!
			created: DateTime!
			reputation: CustomScalar!
			reviews: [Review]
		}
		
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		enum Department {
			COSMETICS,
			ELECTRONICS,
			GROCERIES,
		}

		interface ProductInfo {
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}
		
		scalar BigInt
		
		type Product implements ProductInfo {
			upc: String!
			name: String!
			price: Int!
			worth: BigInt!
			reputation: CustomScalar!
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
			reviews: [Review]
			sales: BigInt!
		}

		input ReviewInput {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}
		
		type Review {
			id: ID!
			created: DateTime!
			body: String!
			author: User!
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}

		type Like {
			id: ID!
			productId: ID!
			userId: ID!
			date: DateTime!
			isDislike: Boolean!
		}

		interface PaymentType {
			name: String!
			email: String!
			date: DateTime!
			amount: BigInt!
			number: String!
			reputation: CustomScalar!
		}
	`

	productAndReviewFederatedSchema = `
		type Query {
			topProducts(first: Int = 5): [Product]
			getReview(id: ID!): Review
		}

		type Mutation {
			createReview(input: ReviewInput): Review
			updateReview(id: ID!, input: ReviewInput): Review
		}

		type Subscription {
			review: Review!
		}
		
		enum Satisfaction {
			UNHAPPY,
			HAPPY,
			NEUTRAL,
		}
		
		scalar CustomScalar
		
		enum Department {
			COSMETICS,
			ELECTRONICS,
			GROCERIES,
		}

		interface ProductInfo {
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}

		scalar BigInt

		type Product implements ProductInfo {
			upc: String!
			name: String!
			price: Int!
			worth: BigInt!
			reputation: CustomScalar!
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
			reviews: [Review]
			sales: BigInt!
		}
		
		scalar DateTime

		input ReviewInput {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}

		type Review {
			id: ID!
			created: DateTime!
			body: String!
			author: User!
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}
		
		extend type User @key(fields: "id") {
			id: ID! @external
			reviews: [Review]
		}

		union AlphaNumeric = Int | String | Float
	`

	productAndExtendsDirectivesFederatedSchema = `
		type Query {
			topProducts(first: Int = 5): [Product]
		}

		enum Satisfaction {
			UNHAPPY,
			HAPPY,
			NEUTRAL,
		}

		scalar CustomScalar

		enum Department {
			COSMETICS,
			ELECTRONICS,
			GROCERIES,
		}

		interface ProductInfo {
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}

		scalar BigInt
		
		type Product implements ProductInfo {
			upc: String!
			name: String!
			price: Int!
			worth: BigInt!
			reputation: CustomScalar!
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}

		union AlphaNumeric = Int | String | Float

		scalar DateTime

		type Comment {
			body: String!
			author: User!
			created: DateTime!
		}

		extend type User @key(fields: "id") {
			id: ID! @external
			comments: [Comment]
		}

		union AlphaNumeric = Int | String | Float

		extend interface PaymentType {
			name: String!
		}
	`
)

func NonIdenticalSharedTypeMergeErrorMessage(typeName string) string {
	return fmt.Sprintf("merge ast: walk: external: the shared type named '%s' must be identical in any subgraphs to federate, locations: [], path: []", typeName)
}

func DuplicateEntityMergeErrorMessage(typeName string) string {
	return fmt.Sprintf("merge ast: walk: external: entities must not be shared types, but the entity named '%s' is duplicated in other subgraph(s), locations: [], path: []", typeName)
}
